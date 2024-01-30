package migration

import (
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpbalpha "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	zondpbv2 "github.com/theQRL/qrysm/v4/proto/zond/v2"
	"google.golang.org/protobuf/proto"
)

// AltairToV1Alpha1SignedBlock converts a v2 SignedBeaconBlockAltair proto to a v1alpha1 proto.
func AltairToV1Alpha1SignedBlock(altairBlk *zondpbv2.SignedBeaconBlockAltair) (*zondpbalpha.SignedBeaconBlockAltair, error) {
	marshaledBlk, err := proto.Marshal(altairBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &zondpbalpha.SignedBeaconBlockAltair{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1Block, nil
}

// BellatrixToV1Alpha1SignedBlock converts a v2 SignedBeaconBlockBellatrix proto to a v1alpha1 proto.
func BellatrixToV1Alpha1SignedBlock(bellatrixBlk *zondpbv2.SignedBeaconBlockBellatrix) (*zondpbalpha.SignedBeaconBlockBellatrix, error) {
	marshaledBlk, err := proto.Marshal(bellatrixBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &zondpbalpha.SignedBeaconBlockBellatrix{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1Block, nil
}

// CapellaToV1Alpha1SignedBlock converts a v2 SignedBeaconBlockCapella proto to a v1alpha1 proto.
func CapellaToV1Alpha1SignedBlock(capellaBlk *zondpbv2.SignedBeaconBlockCapella) (*zondpbalpha.SignedBeaconBlockCapella, error) {
	marshaledBlk, err := proto.Marshal(capellaBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &zondpbalpha.SignedBeaconBlockCapella{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1Block, nil
}

// DenebToV1Alpha1SignedBlock converts a v2 SignedBeaconBlockDeneb proto to a v1alpha1 proto.
func DenebToV1Alpha1SignedBlock(denebBlk *zondpbv2.SignedBeaconBlockDeneb) (*zondpbalpha.SignedBeaconBlockDeneb, error) {
	marshaledBlk, err := proto.Marshal(denebBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &zondpbalpha.SignedBeaconBlockDeneb{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1Block, nil
}

// V2BeaconBlockDenebToV1Alpha1 converts a v2 Deneb beacon block to a v1alpha1
// Deneb block.
func V2BeaconBlockDenebToV1Alpha1(v2block *zondpbv2.BeaconBlockDeneb) (*zondpbalpha.BeaconBlockDeneb, error) {
	marshaledBlk, err := proto.Marshal(v2block)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1block := &zondpbalpha.BeaconBlockDeneb{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1block, nil
}

// BlindedBellatrixToV1Alpha1SignedBlock converts a v2 SignedBlindedBeaconBlockBellatrix proto to a v1alpha1 proto.
func BlindedBellatrixToV1Alpha1SignedBlock(bellatrixBlk *zondpbv2.SignedBlindedBeaconBlockBellatrix) (*zondpbalpha.SignedBlindedBeaconBlockBellatrix, error) {
	marshaledBlk, err := proto.Marshal(bellatrixBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &zondpbalpha.SignedBlindedBeaconBlockBellatrix{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1Block, nil
}

// BlindedCapellaToV1Alpha1SignedBlock converts a v2 SignedBlindedBeaconBlockCapella proto to a v1alpha1 proto.
func BlindedCapellaToV1Alpha1SignedBlock(capellaBlk *zondpbv2.SignedBlindedBeaconBlockCapella) (*zondpbalpha.SignedBlindedBeaconBlockCapella, error) {
	marshaledBlk, err := proto.Marshal(capellaBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &zondpbalpha.SignedBlindedBeaconBlockCapella{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1Block, nil
}

// BlindedDenebToV1Alpha1SignedBlock converts a v2 SignedBlindedBeaconBlockDeneb proto to a v1alpha1 proto.
func BlindedDenebToV1Alpha1SignedBlock(denebBlk *zondpbv2.SignedBlindedBeaconBlockDeneb) (*zondpbalpha.SignedBlindedBeaconBlockDeneb, error) {
	marshaledBlk, err := proto.Marshal(denebBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &zondpbalpha.SignedBlindedBeaconBlockDeneb{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1Block, nil
}

// SignedBlindedBlobsToV1Alpha1SignedBlindedBlobs converts an array of v2 SignedBlindedBlobSidecar objects to its v1alpha1 equivalent.
func SignedBlindedBlobsToV1Alpha1SignedBlindedBlobs(sidecars []*zondpbv2.SignedBlindedBlobSidecar) []*zondpbalpha.SignedBlindedBlobSidecar {
	result := make([]*zondpbalpha.SignedBlindedBlobSidecar, len(sidecars))
	for i, sc := range sidecars {
		result[i] = &zondpbalpha.SignedBlindedBlobSidecar{
			Message: &zondpbalpha.BlindedBlobSidecar{
				BlockRoot:       bytesutil.SafeCopyBytes(sc.Message.BlockRoot),
				Index:           sc.Message.Index,
				Slot:            sc.Message.Slot,
				BlockParentRoot: bytesutil.SafeCopyBytes(sc.Message.BlockParentRoot),
				ProposerIndex:   sc.Message.ProposerIndex,
				BlobRoot:        bytesutil.SafeCopyBytes(sc.Message.BlobRoot),
				KzgCommitment:   bytesutil.SafeCopyBytes(sc.Message.KzgCommitment),
				KzgProof:        bytesutil.SafeCopyBytes(sc.Message.KzgProof),
			},
			Signature: bytesutil.SafeCopyBytes(sc.Signature),
		}
	}
	return result
}

// SignedBlobsToV1Alpha1SignedBlobs converts an array of v2 SignedBlobSidecar objects to its v1alpha1 equivalent.
func SignedBlobsToV1Alpha1SignedBlobs(sidecars []*zondpbv2.SignedBlobSidecar) []*zondpbalpha.SignedBlobSidecar {
	result := make([]*zondpbalpha.SignedBlobSidecar, len(sidecars))
	for i, sc := range sidecars {
		result[i] = &zondpbalpha.SignedBlobSidecar{
			Message: &zondpbalpha.BlobSidecar{
				BlockRoot:       bytesutil.SafeCopyBytes(sc.Message.BlockRoot),
				Index:           sc.Message.Index,
				Slot:            sc.Message.Slot,
				BlockParentRoot: bytesutil.SafeCopyBytes(sc.Message.BlockParentRoot),
				ProposerIndex:   sc.Message.ProposerIndex,
				Blob:            bytesutil.SafeCopyBytes(sc.Message.Blob),
				KzgCommitment:   bytesutil.SafeCopyBytes(sc.Message.KzgCommitment),
				KzgProof:        bytesutil.SafeCopyBytes(sc.Message.KzgProof),
			},
			Signature: bytesutil.SafeCopyBytes(sc.Signature),
		}
	}
	return result
}

// DenebBlockContentsToV1Alpha1 converts signed deneb block contents to signed beacon block and blobs deneb
func DenebBlockContentsToV1Alpha1(blockcontents *zondpbv2.SignedBeaconBlockContentsDeneb) (*zondpbalpha.SignedBeaconBlockAndBlobsDeneb, error) {
	block, err := DenebToV1Alpha1SignedBlock(blockcontents.SignedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert block")
	}
	blobs := SignedBlobsToV1Alpha1SignedBlobs(blockcontents.SignedBlobSidecars)
	return &zondpbalpha.SignedBeaconBlockAndBlobsDeneb{Block: block, Blobs: blobs}, nil
}

// BlindedDenebBlockContentsToV1Alpha1 converts signed blinded deneb block contents to signed blinded beacon block and blobs deneb
func BlindedDenebBlockContentsToV1Alpha1(blockcontents *zondpbv2.SignedBlindedBeaconBlockContentsDeneb) (*zondpbalpha.SignedBlindedBeaconBlockAndBlobsDeneb, error) {
	block, err := BlindedDenebToV1Alpha1SignedBlock(blockcontents.SignedBlindedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert block")
	}
	blobs := SignedBlindedBlobsToV1Alpha1SignedBlindedBlobs(blockcontents.SignedBlindedBlobSidecars)
	return &zondpbalpha.SignedBlindedBeaconBlockAndBlobsDeneb{SignedBlindedBlock: block, SignedBlindedBlobSidecars: blobs}, nil
}
