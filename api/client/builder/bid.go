package builder

import (
	ssz "github.com/prysmaticlabs/fastssz"
	consensus_types "github.com/theQRL/qrysm/v4/consensus-types"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
)

// SignedBid is an interface describing the method set of a signed builder bid.
type SignedBid interface {
	Message() (Bid, error)
	Signature() []byte
	Version() int
	IsNil() bool
}

// Bid is an interface describing the method set of a builder bid.
type Bid interface {
	Header() (interfaces.ExecutionData, error)
	Value() []byte
	Pubkey() []byte
	Version() int
	IsNil() bool
	HashTreeRoot() ([32]byte, error)
	HashTreeRootWith(hh *ssz.Hasher) error
}

type signedBuilderBidCapella struct {
	p *zondpb.SignedBuilderBidCapella
}

// WrappedSignedBuilderBidCapella is a constructor which wraps a protobuf signed bit into an interface.
func WrappedSignedBuilderBidCapella(p *zondpb.SignedBuilderBidCapella) (SignedBid, error) {
	w := signedBuilderBidCapella{p: p}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// Message --
func (b signedBuilderBidCapella) Message() (Bid, error) {
	return WrappedBuilderBidCapella(b.p.Message)
}

// Signature --
func (b signedBuilderBidCapella) Signature() []byte {
	return b.p.Signature
}

// Version --
func (b signedBuilderBidCapella) Version() int {
	return version.Capella
}

// IsNil --
func (b signedBuilderBidCapella) IsNil() bool {
	return b.p == nil
}

type builderBidCapella struct {
	p *zondpb.BuilderBidCapella
}

// WrappedBuilderBidCapella is a constructor which wraps a protobuf bid into an interface.
func WrappedBuilderBidCapella(p *zondpb.BuilderBidCapella) (Bid, error) {
	w := builderBidCapella{p: p}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// Header returns the execution data interface.
func (b builderBidCapella) Header() (interfaces.ExecutionData, error) {
	// We have to convert big endian to little endian because the value is coming from the execution layer.
	return blocks.WrappedExecutionPayloadHeaderCapella(b.p.Header, blocks.PayloadValueToGwei(b.p.Value))
}

// Version --
func (b builderBidCapella) Version() int {
	return version.Capella
}

// Value --
func (b builderBidCapella) Value() []byte {
	return b.p.Value
}

// Pubkey --
func (b builderBidCapella) Pubkey() []byte {
	return b.p.Pubkey
}

// IsNil --
func (b builderBidCapella) IsNil() bool {
	return b.p == nil
}

// HashTreeRoot --
func (b builderBidCapella) HashTreeRoot() ([32]byte, error) {
	return b.p.HashTreeRoot()
}

// HashTreeRootWith --
func (b builderBidCapella) HashTreeRootWith(hh *ssz.Hasher) error {
	return b.p.HashTreeRootWith(hh)
}
