package metadata

import (
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/theQRL/go-bitfield"
	pb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// Metadata returns the interface of a p2p metadata type.
type Metadata interface {
	SequenceNumber() uint64
	AttnetsBitfield() bitfield.Bitvector64
	SyncnetsBitfield() bitfield.Bitvector4
	InnerObject() any
	IsNil() bool
	Copy() Metadata
	ssz.Marshaler
	ssz.Unmarshaler
	MetadataObjV1() *pb.MetaDataV1
	Version() int
}
