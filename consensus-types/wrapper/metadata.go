package wrapper

import (
	"github.com/theQRL/go-bitfield"
	pb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/metadata"
	"github.com/theQRL/qrysm/runtime/version"
	"google.golang.org/protobuf/proto"
)

// MetadataV1 is a convenience wrapper around our metadata v2 protobuf object.
type MetadataV1 struct {
	md *pb.MetaDataV1
}

// WrappedMetadataV1 wrappers around the provided protobuf object.
func WrappedMetadataV1(md *pb.MetaDataV1) MetadataV1 {
	return MetadataV1{md: md}
}

// SequenceNumber returns the sequence number from the metadata.
func (m MetadataV1) SequenceNumber() uint64 {
	return m.md.SeqNumber
}

// AttnetsBitfield returns the bitfield stored in the metadata.
func (m MetadataV1) AttnetsBitfield() bitfield.Bitvector64 {
	return m.md.Attnets
}

// SyncnetsBitfield returns the sync committee subnet bitfield stored in the metadata.
func (m MetadataV1) SyncnetsBitfield() bitfield.Bitvector4 {
	return m.md.Syncnets
}

// InnerObject returns the underlying metadata protobuf structure.
func (m MetadataV1) InnerObject() any {
	return m.md
}

// IsNil checks for the nilness of the underlying object.
func (m MetadataV1) IsNil() bool {
	return m.md == nil
}

// Copy performs a full copy of the underlying metadata object.
func (m MetadataV1) Copy() metadata.Metadata {
	return WrappedMetadataV1(proto.Clone(m.md).(*pb.MetaDataV1))
}

// MarshalSSZ marshals the underlying metadata object
// into its serialized form.
func (m MetadataV1) MarshalSSZ() ([]byte, error) {
	return m.md.MarshalSSZ()
}

// MarshalSSZTo marshals the underlying metadata object
// into its serialized form into the provided byte buffer.
func (m MetadataV1) MarshalSSZTo(dst []byte) ([]byte, error) {
	return m.md.MarshalSSZTo(dst)
}

// SizeSSZ returns the serialized size of the metadata object.
func (m MetadataV1) SizeSSZ() int {
	return m.md.SizeSSZ()
}

// UnmarshalSSZ unmarshals the provided byte buffer into
// the underlying metadata object.
func (m MetadataV1) UnmarshalSSZ(buf []byte) error {
	return m.md.UnmarshalSSZ(buf)
}

// MetadataObjV1 returns the inner metadata object in its type
// specified form. If it doesn't exist then we return nothing.
func (m MetadataV1) MetadataObjV1() *pb.MetaDataV1 {
	return m.md
}

// Version returns the fork version of the underlying object.
func (_ MetadataV1) Version() int {
	return version.Zond
}
