// Package randao implements the RANDAO hash-onion commit-reveal scheme used by
// QRL in place of Ethereum's signature-based RANDAO.
//
// ML-DSA-87 signatures are not unique: a signer can produce arbitrarily many
// valid signatures for the same message, so hashing a signature does not yield
// a value the signer cannot bias. Instead each validator commits, in its
// deposit, to the top of a hash chain ("onion")
//
//	x_0 = SHA-256(domainTag || mlDSASeed)
//	x_{j+1} = SHA-256(x_j)
//	commitment = x_L
//
// and every block it proposes reveals the pre-image of the commitment currently
// recorded in the beacon state: the block's randao_reveal r must satisfy
// SHA-256(r) == validator.randao_commitment, after which r becomes the new
// commitment and is XOR-ed into the RANDAO mix. The proposer therefore has no
// choice in the value it contributes, only in whether it publishes the block at
// all, which restores exactly the bias bound of Ethereum's BLS-based RANDAO.
//
// The onion length L is a validator-side choice, not a consensus constant: the
// state transition only ever checks a single hash. A validator locates its
// current position by scanning forward from x_0, so it never has to know which
// L the deposit was made with.
package randao

import (
	"crypto/sha256"
	"errors"
	"sync"

	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
)

const (
	// DefaultLayers is the onion length used by the deposit CLI and the
	// validator client: 2^20 reveals, i.e. 2^20 block proposals per validator.
	DefaultLayers uint64 = 1 << 20

	// checkpointInterval is the spacing of the cached intermediate layers.
	// Deriving any layer costs at most checkpointInterval hashes.
	checkpointInterval uint64 = 1 << 10
)

// domainTag separates the onion origin from any other use of the ML-DSA seed.
var domainTag = []byte("qrl-randao-onion-v1")

var (
	// ErrExhausted is returned when the commitment is x_0, i.e. the onion has no
	// layers left to reveal.
	ErrExhausted = errors.New("randao onion exhausted: no layer left to reveal")
	// ErrUnknownCommitment is returned when the commitment is not a layer of
	// this onion (wrong seed, or a deposit made with a longer onion).
	ErrUnknownCommitment = errors.New("commitment is not a layer of this randao onion")
	// ErrInvalidLayers is returned by NewOnion for a zero layer count.
	ErrInvalidLayers = errors.New("randao onion must have at least one layer")
)

// Origin derives x_0 from a validator's ML-DSA-87 seed.
func Origin(mlDSASeed []byte) [fieldparams.RandaoCommitmentLength]byte {
	buf := make([]byte, 0, len(domainTag)+len(mlDSASeed))
	buf = append(buf, domainTag...)
	buf = append(buf, mlDSASeed...)
	return sha256.Sum256(buf)
}

// Next returns the layer above x, i.e. SHA-256(x).
func Next(x [fieldparams.RandaoCommitmentLength]byte) [fieldparams.RandaoCommitmentLength]byte {
	return sha256.Sum256(x[:])
}

// Verify reports whether reveal is the pre-image of commitment.
func Verify(reveal, commitment [fieldparams.RandaoCommitmentLength]byte) bool {
	return Next(reveal) == commitment
}

// Commitment returns the top layer x_layers of the onion derived from mlDSASeed
// without retaining any intermediate state. Cost: layers hashes.
func Commitment(mlDSASeed []byte, layers uint64) [fieldparams.RandaoCommitmentLength]byte {
	x := Origin(mlDSASeed)
	for range layers {
		x = Next(x)
	}
	return x
}

// Onion holds the checkpoints of one validator's hash chain and can produce
// the reveal matching any commitment that is a layer of the chain.
type Onion struct {
	origin      [fieldparams.RandaoCommitmentLength]byte
	layers      uint64
	checkpoints [][fieldparams.RandaoCommitmentLength]byte // checkpoints[k] == x_{k*checkpointInterval}

	mu       sync.Mutex
	posKnown bool
	pos      uint64 // index j of the last commitment located, used as a hint
}

// NewOnion builds the onion of the given length from an ML-DSA-87 seed.
// Cost: layers hashes (about a quarter of a second for DefaultLayers).
func NewOnion(mlDSASeed []byte, layers uint64) (*Onion, error) {
	if layers == 0 {
		return nil, ErrInvalidLayers
	}
	o := &Onion{
		origin:      Origin(mlDSASeed),
		layers:      layers,
		checkpoints: make([][fieldparams.RandaoCommitmentLength]byte, 0, layers/checkpointInterval+1),
	}
	x := o.origin
	for j := uint64(0); j <= layers; j++ {
		if j%checkpointInterval == 0 {
			o.checkpoints = append(o.checkpoints, x)
		}
		if j < layers {
			x = Next(x)
		}
	}
	return o, nil
}

// Layers returns the onion length L.
func (o *Onion) Layers() uint64 {
	return o.layers
}

// Commitment returns the top layer x_L, the value to put in the deposit.
func (o *Onion) Commitment() [fieldparams.RandaoCommitmentLength]byte {
	x, _ := o.Layer(o.layers)
	return x
}

// Layer returns x_j for 0 <= j <= L. Cost: at most checkpointInterval hashes.
func (o *Onion) Layer(j uint64) ([fieldparams.RandaoCommitmentLength]byte, error) {
	if j > o.layers {
		return [fieldparams.RandaoCommitmentLength]byte{}, errors.New("layer index beyond onion length")
	}
	k := j / checkpointInterval
	x := o.checkpoints[k]
	for i := k * checkpointInterval; i < j; i++ {
		x = Next(x)
	}
	return x, nil
}

// Reveal returns the pre-image x_{j-1} of the given commitment x_j. It first
// tries the neighbourhood of the last located position (the common case after
// a proposal, or after a short reorg) and falls back to a full forward scan.
func (o *Onion) Reveal(commitment [fieldparams.RandaoCommitmentLength]byte) ([fieldparams.RandaoCommitmentLength]byte, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	j, ok := o.locate(commitment)
	if !ok {
		return [fieldparams.RandaoCommitmentLength]byte{}, ErrUnknownCommitment
	}
	if j == 0 {
		return [fieldparams.RandaoCommitmentLength]byte{}, ErrExhausted
	}
	reveal, err := o.Layer(j - 1)
	if err != nil {
		return [fieldparams.RandaoCommitmentLength]byte{}, err
	}
	// After this reveal is included the commitment becomes x_{j-1}.
	o.posKnown, o.pos = true, j-1
	return reveal, nil
}

// locate finds j such that x_j == commitment. Caller holds o.mu.
func (o *Onion) locate(commitment [fieldparams.RandaoCommitmentLength]byte) (uint64, bool) {
	if o.posKnown {
		// Try the hint and a few layers around it: the same position (the
		// proposal was not yet included, or was orphaned), one below (it was
		// included), and a few above (a deeper reorg).
		const window = 4
		lo := uint64(0)
		if o.pos > window {
			lo = o.pos - window
		}
		hi := min(o.pos+window, o.layers)
		for j := lo; j <= hi; j++ {
			if x, err := o.Layer(j); err == nil && x == commitment {
				return j, true
			}
		}
	}
	x := o.origin
	for j := uint64(0); j <= o.layers; j++ {
		if x == commitment {
			return j, true
		}
		x = Next(x)
	}
	return 0, false
}
