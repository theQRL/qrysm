package util

import (
	"fmt"

	"github.com/theQRL/go-qrllib/dilithium"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	v2 "github.com/theQRL/qrysm/v4/proto/zond/v2"
)

// NewBeaconBlockBellatrix creates a beacon block with minimum marshalable fields.
func NewBeaconBlockBellatrix() *zondpb.SignedBeaconBlockBellatrix {
	return HydrateSignedBeaconBlockBellatrix(&zondpb.SignedBeaconBlockBellatrix{})
}

// NewBlindedBeaconBlockBellatrix creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockBellatrix() *zondpb.SignedBlindedBeaconBlockBellatrix {
	return HydrateSignedBlindedBeaconBlockBellatrix(&zondpb.SignedBlindedBeaconBlockBellatrix{})
}

// NewBlindedBeaconBlockBellatrixV2 creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockBellatrixV2() *v2.SignedBlindedBeaconBlockBellatrix {
	return HydrateV2SignedBlindedBeaconBlockBellatrix(&v2.SignedBlindedBeaconBlockBellatrix{})
}

// NewBeaconBlockCapella creates a beacon block with minimum marshalable fields.
func NewBeaconBlockCapella() *zondpb.SignedBeaconBlockCapella {
	return HydrateSignedBeaconBlockCapella(&zondpb.SignedBeaconBlockCapella{})
}

// NewBlindedBeaconBlockCapella creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockCapella() *zondpb.SignedBlindedBeaconBlockCapella {
	return HydrateSignedBlindedBeaconBlockCapella(&zondpb.SignedBlindedBeaconBlockCapella{})
}

// NewBeaconBlockDeneb creates a beacon block with minimum marshalable fields.
func NewBeaconBlockDeneb() *zondpb.SignedBeaconBlockDeneb {
	return HydrateSignedBeaconBlockDeneb(&zondpb.SignedBeaconBlockDeneb{})
}

// NewBlindedBeaconBlockDeneb creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockDeneb() *zondpb.SignedBlindedBeaconBlockDeneb {
	return HydrateSignedBlindedBeaconBlockDeneb(&zondpb.SignedBlindedBeaconBlockDeneb{})
}

// NewBlindedBlobSidecar creates a signed blinded blob sidecar with minimum marshalable fields.
func NewBlindedBlobSidecar() *zondpb.SignedBlindedBlobSidecar {
	return HydrateSignedBlindedBlobSidecar(&zondpb.SignedBlindedBlobSidecar{})
}

// NewBlindedBeaconBlockCapellaV2 creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockCapellaV2() *v2.SignedBlindedBeaconBlockCapella {
	return HydrateV2SignedBlindedBeaconBlockCapella(&v2.SignedBlindedBeaconBlockCapella{})
}

// NewBeaconBlockContentsDeneb creates a beacon block content including blobs with minimum marshalable fields.
func NewBeaconBlockContentsDeneb(numOfBlobs uint64) (*v2.SignedBeaconBlockContentsDeneb, error) {
	if numOfBlobs > fieldparams.MaxBlobsPerBlock {
		return nil, fmt.Errorf("declared too many blobs: %v", numOfBlobs)
	}
	blobs := make([]*v2.SignedBlobSidecar, numOfBlobs)
	for i := range blobs {
		blobs[i] = &v2.SignedBlobSidecar{
			Message: &v2.BlobSidecar{
				BlockRoot:       make([]byte, fieldparams.RootLength),
				Index:           0,
				Slot:            0,
				BlockParentRoot: make([]byte, fieldparams.RootLength),
				ProposerIndex:   0,
				Blob:            make([]byte, fieldparams.BlobLength),
				KzgCommitment:   make([]byte, dilithium.CryptoPublicKeyBytes),
				KzgProof:        make([]byte, dilithium.CryptoPublicKeyBytes),
			},
			Signature: make([]byte, dilithium.CryptoBytes),
		}
	}
	return &v2.SignedBeaconBlockContentsDeneb{
		SignedBlock:        HydrateV2SignedBeaconBlockDeneb(&v2.SignedBeaconBlockDeneb{}),
		SignedBlobSidecars: blobs,
	}, nil
}

// NewBlindedBeaconBlockContentsDeneb creates a blinded beacon block content including blobs with minimum marshalable fields.
func NewBlindedBeaconBlockContentsDeneb(numOfBlobs uint64) (*v2.SignedBlindedBeaconBlockContentsDeneb, error) {
	if numOfBlobs > fieldparams.MaxBlobsPerBlock {
		return nil, fmt.Errorf("declared too many blobs: %v", numOfBlobs)
	}
	blobs := make([]*v2.SignedBlindedBlobSidecar, numOfBlobs)
	for i := range blobs {
		blobs[i] = &v2.SignedBlindedBlobSidecar{
			Message: &v2.BlindedBlobSidecar{
				BlockRoot:       make([]byte, fieldparams.RootLength),
				Index:           0,
				Slot:            0,
				BlockParentRoot: make([]byte, fieldparams.RootLength),
				ProposerIndex:   0,
				BlobRoot:        make([]byte, fieldparams.RootLength),
				KzgCommitment:   make([]byte, dilithium.CryptoPublicKeyBytes),
				KzgProof:        make([]byte, dilithium.CryptoPublicKeyBytes),
			},
			Signature: make([]byte, dilithium.CryptoBytes),
		}
	}
	return &v2.SignedBlindedBeaconBlockContentsDeneb{
		SignedBlindedBlock:        HydrateV2SignedBlindedBeaconBlockDeneb(&v2.SignedBlindedBeaconBlockDeneb{}),
		SignedBlindedBlobSidecars: blobs,
	}, nil
}
