package builder

import (
	ssz "github.com/prysmaticlabs/fastssz"
	consensus_types "github.com/theQRL/qrysm/consensus-types"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
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

type signedBuilderBidZond struct {
	p *qrysmpb.SignedBuilderBidZond
}

// WrappedSignedBuilderBidZond is a constructor which wraps a protobuf signed bit into an interface.
func WrappedSignedBuilderBidZond(p *qrysmpb.SignedBuilderBidZond) (SignedBid, error) {
	w := signedBuilderBidZond{p: p}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// Message --
func (b signedBuilderBidZond) Message() (Bid, error) {
	return WrappedBuilderBidZond(b.p.Message)
}

// Signature --
func (b signedBuilderBidZond) Signature() []byte {
	return b.p.Signature
}

// Version --
func (b signedBuilderBidZond) Version() int {
	return version.Zond
}

// IsNil --
func (b signedBuilderBidZond) IsNil() bool {
	return b.p == nil
}

type builderBidZond struct {
	p *qrysmpb.BuilderBidZond
}

// WrappedBuilderBidZond is a constructor which wraps a protobuf bid into an interface.
func WrappedBuilderBidZond(p *qrysmpb.BuilderBidZond) (Bid, error) {
	w := builderBidZond{p: p}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// Header returns the execution data interface.
func (b builderBidZond) Header() (interfaces.ExecutionData, error) {
	// We have to convert big endian to little endian because the value is coming from the execution layer.
	return blocks.WrappedExecutionPayloadHeaderZond(b.p.Header, blocks.PayloadValueToShor(b.p.Value))
}

// Version --
func (b builderBidZond) Version() int {
	return version.Zond
}

// Value --
func (b builderBidZond) Value() []byte {
	return b.p.Value
}

// Pubkey --
func (b builderBidZond) Pubkey() []byte {
	return b.p.Pubkey
}

// IsNil --
func (b builderBidZond) IsNil() bool {
	return b.p == nil
}

// HashTreeRoot --
func (b builderBidZond) HashTreeRoot() ([32]byte, error) {
	return b.p.HashTreeRoot()
}

// HashTreeRootWith --
func (b builderBidZond) HashTreeRootWith(hh *ssz.Hasher) error {
	return b.p.HashTreeRootWith(hh)
}
