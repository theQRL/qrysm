package ml_dsa_87t

import (
	"errors"
	"fmt"
	"runtime"

	pkgerrors "github.com/pkg/errors"
	"github.com/theQRL/go-qrllib/wallet/ml_dsa_87"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87/common"
	"golang.org/x/sync/errgroup"
)

var errSignatureVerificationFailed = errors.New("signature verification failed")

// Signature used in the ML-DSA-87 signature scheme.
type Signature struct {
	s *[field_params.MLDSA87SignatureLength]uint8
}

func SignatureFromBytes(sig []byte) (common.Signature, error) {
	if len(sig) != field_params.MLDSA87SignatureLength {
		return nil, fmt.Errorf("signature must be %d bytes", field_params.MLDSA87SignatureLength)
	}
	var signature [field_params.MLDSA87SignatureLength]uint8
	copy(signature[:], sig)
	return &Signature{s: &signature}, nil
}

func (s *Signature) Verify(pubKey common.PublicKey, msg []byte) bool {
	if s == nil || s.s == nil {
		return false
	}
	key, ok := pubKey.(*PublicKey)
	if !ok || key == nil || key.p == nil {
		return false
	}
	sig := *s.s
	d, err := ml_dsa_87.NewMLDSA87Descriptor()
	if err != nil {
		return false
	}
	return ml_dsa_87.Verify(msg, sig[:], key.p, d)
}

func VerifySignature(sig []byte, msg [32]byte, pubKey common.PublicKey) (bool, error) {
	rSig, err := SignatureFromBytes(sig)
	if err != nil {
		return false, err
	}
	return rSig.Verify(pubKey, msg[:]), nil
}

// ValidateSignatureBatch checks batch dimensions and public-key objects without
// verifying signatures. An entirely empty batch is structurally valid.
func ValidateSignatureBatch(sigsBatches [][][]byte, msgs [][32]byte, pubKeysBatches [][]common.PublicKey) error {
	var (
		lenSigsBatches    = len(sigsBatches)
		lenPubKeysBatches = len(pubKeysBatches)
		lenMsgsBatches    = len(msgs)
	)

	if lenSigsBatches != lenPubKeysBatches || lenSigsBatches != lenMsgsBatches {
		return pkgerrors.Errorf("provided signatures batches, pubkeys batches and messages have differing lengths. SB: %d, PB: %d, M: %d",
			lenSigsBatches, lenPubKeysBatches, lenMsgsBatches)
	}

	for i := range lenMsgsBatches {
		if len(sigsBatches[i]) != len(pubKeysBatches[i]) {
			return pkgerrors.Errorf("provided signatures, pubkeys have differing lengths. S: %d, P: %d, Batch: %d",
				len(sigsBatches[i]), len(pubKeysBatches[i]), i)
		}
		if len(sigsBatches[i]) == 0 {
			return pkgerrors.Errorf("signature group %d is empty", i)
		}
		for j, pubKey := range pubKeysBatches[i] {
			key, ok := pubKey.(*PublicKey)
			if !ok || key == nil || key.p == nil {
				return pkgerrors.Errorf("invalid public key at batch %d, index %d", i, j)
			}
		}
	}
	return nil
}

func VerifyMultipleSignatures(sigsBatches [][][]byte, msgs [][32]byte, pubKeysBatches [][]common.PublicKey) (bool, error) {
	return VerifyMultipleSignaturesWithReporter(sigsBatches, msgs, pubKeysBatches, nil)
}

// VerifyMultipleSignaturesWithReporter verifies each signature once and reports
// failures during that same pass. report, when non-nil, must be safe for concurrent
// calls and must not modify the batch. A nil reported error means the signature was
// correctly sized but cryptographically invalid. Structural batch errors are
// returned before any signatures are verified or reported.
func VerifyMultipleSignaturesWithReporter(sigsBatches [][][]byte, msgs [][32]byte, pubKeysBatches [][]common.PublicKey, report func(batchIndex, signatureIndex int, err error)) (bool, error) {
	// Validate every group before starting workers, so malformed later groups
	// cannot leave earlier verifications running after an error is returned.
	if err := ValidateSignatureBatch(sigsBatches, msgs, pubKeysBatches); err != nil {
		return false, err
	}
	if len(sigsBatches) == 0 {
		return false, nil
	}

	maxProcs := max(runtime.GOMAXPROCS(0)-1, 1)
	grp := errgroup.Group{}
	grp.SetLimit(maxProcs)

	for i := range msgs {
		index := i

		for j := range sigsBatches[index] {
			jCopy := j

			grp.Go(func() error {
				ok, err := VerifySignature(sigsBatches[index][jCopy], msgs[index], pubKeysBatches[index][jCopy])
				if report != nil && (err != nil || !ok) {
					report(index, jCopy, err)
				}
				if err != nil {
					return err
				}
				if !ok {
					return errSignatureVerificationFailed
				}

				return nil
			})
		}
	}

	if err := grp.Wait(); err != nil {
		if pkgerrors.Is(err, errSignatureVerificationFailed) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// Marshal returns the signature bytes, or nil for a nil or uninitialized
// signature so that callers never dereference a missing value.
func (s *Signature) Marshal() []byte {
	if s == nil || s.s == nil {
		return nil
	}
	return s.s[:]
}

// Copy returns an independent copy of the signature, or nil for a nil or
// uninitialized signature.
func (s *Signature) Copy() common.Signature {
	if s == nil || s.s == nil {
		return nil
	}
	sign := *s.s
	return &Signature{s: &sign}
}
