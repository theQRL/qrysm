package util

import (
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

// HydrateBlobSidecar hydrates a blob sidecar with correct field length sizes
// to comply with SSZ marshalling and unmarshalling rules.
func HydrateBlobSidecar(b *zondpb.BlobSidecar) *zondpb.BlobSidecar {
	if b.BlockRoot == nil {
		b.BlockRoot = make([]byte, fieldparams.RootLength)
	}
	if b.BlockParentRoot == nil {
		b.BlockParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.Blob == nil {
		b.Blob = make([]byte, fieldparams.BlobLength)
	}
	if b.KzgCommitment == nil {
		b.KzgCommitment = make([]byte, dilithium2.CryptoPublicKeyBytes)
	}
	if b.KzgProof == nil {
		b.KzgProof = make([]byte, dilithium2.CryptoPublicKeyBytes)
	}
	return b
}

// HydrateSignedBlindedBlobSidecar hydrates a signed blinded blob sidecar with correct field length sizes
// to comply with SSZ marshalling and unmarshalling rules.
func HydrateSignedBlindedBlobSidecar(b *zondpb.SignedBlindedBlobSidecar) *zondpb.SignedBlindedBlobSidecar {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Message = HydrateBlindedBlobSidecar(b.Message)
	return b
}

// HydrateBlindedBlobSidecar hydrates a blinded blob sidecar with correct field length sizes
// to comply with SSZ marshalling and unmarshalling rules.
func HydrateBlindedBlobSidecar(b *zondpb.BlindedBlobSidecar) *zondpb.BlindedBlobSidecar {
	if b.BlockRoot == nil {
		b.BlockRoot = make([]byte, fieldparams.RootLength)
	}
	if b.BlockParentRoot == nil {
		b.BlockParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.KzgCommitment == nil {
		b.KzgCommitment = make([]byte, dilithium2.CryptoPublicKeyBytes)
	}
	if b.KzgProof == nil {
		b.KzgProof = make([]byte, dilithium2.CryptoPublicKeyBytes)
	}
	if b.BlobRoot == nil {
		b.BlobRoot = make([]byte, fieldparams.RootLength)
	}
	return b
}
