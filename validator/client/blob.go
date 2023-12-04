package client

import (
	"context"

	"github.com/pkg/errors"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/v4/config/params"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/v4/time/slots"
)

func (v *validator) signBlob(ctx context.Context, blob *zondpb.BlobSidecar, pubKey [dilithium2.CryptoPublicKeyBytes]byte) ([]byte, error) {
	epoch := slots.ToEpoch(blob.Slot)
	domain, err := v.domainData(ctx, epoch, params.BeaconConfig().DomainBlobSidecar[:])
	if err != nil {
		return nil, errors.Wrap(err, domainDataErr)
	}
	if domain == nil {
		return nil, errors.New(domainDataErr)
	}
	sr, err := signing.ComputeSigningRoot(blob, domain.SignatureDomain)
	if err != nil {
		return nil, errors.Wrap(err, signingRootErr)
	}
	sig, err := v.keyManager.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     sr[:],
		SignatureDomain: domain.SignatureDomain,
		Object:          &validatorpb.SignRequest_Blob{Blob: blob},
		SigningSlot:     blob.Slot,
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not sign block proposal")
	}
	return sig.Marshal(), nil
}

// signBlindBlob signs a given blinded blob sidecar for a specific slot.
// It calculates the signing root for the blob and then uses the key manager to produce the signature.
func (v *validator) signBlindBlob(ctx context.Context, blob *zondpb.BlindedBlobSidecar, pubKey [dilithium2.CryptoPublicKeyBytes]byte) ([]byte, error) {
	epoch := slots.ToEpoch(blob.Slot)

	// Retrieve domain data specific to the epoch and `DOMAIN_BLOB_SIDECAR`.
	domain, err := v.domainData(ctx, epoch, params.BeaconConfig().DomainBlobSidecar[:])
	if err != nil {
		return nil, errors.Wrap(err, domainDataErr)
	}
	if domain == nil {
		return nil, errors.New(domainDataErr)
	}

	// Compute the signing root for the blob.
	sr, err := signing.ComputeSigningRoot(blob, domain.SignatureDomain)
	if err != nil {
		return nil, errors.Wrap(err, signingRootErr)
	}

	// Create a sign request and use the key manager to sign it.
	sig, err := v.keyManager.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     sr[:],
		SignatureDomain: domain.SignatureDomain,
		Object:          &validatorpb.SignRequest_BlindedBlob{BlindedBlob: blob},
		SigningSlot:     blob.Slot,
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not sign blind blob sidecar")
	}
	return sig.Marshal(), nil
}

// signDenebBlobs signs an array of Deneb blobs using the provided public key.
func (v *validator) signDenebBlobs(ctx context.Context, blobs []*zondpb.BlobSidecar, pubKey [dilithium2.CryptoPublicKeyBytes]byte) ([]*zondpb.SignedBlobSidecar, error) {
	signedBlobs := make([]*zondpb.SignedBlobSidecar, len(blobs))
	for i, blob := range blobs {
		blobSig, err := v.signBlob(ctx, blob, pubKey)
		if err != nil {
			return nil, err
		}
		signedBlobs[i] = &zondpb.SignedBlobSidecar{
			Message:   blob,
			Signature: blobSig,
		}
	}
	return signedBlobs, nil
}

// signBlindedDenebBlobs signs an array of blinded Deneb blobs using the provided public key.
func (v *validator) signBlindedDenebBlobs(ctx context.Context, blobs []*zondpb.BlindedBlobSidecar, pubKey [dilithium2.CryptoPublicKeyBytes]byte) ([]*zondpb.SignedBlindedBlobSidecar, error) {
	signedBlindBlobs := make([]*zondpb.SignedBlindedBlobSidecar, len(blobs))
	for i, blob := range blobs {
		blobSig, err := v.signBlindBlob(ctx, blob, pubKey)
		if err != nil {
			return nil, err
		}
		signedBlindBlobs[i] = &zondpb.SignedBlindedBlobSidecar{
			Message:   blob,
			Signature: blobSig,
		}
	}
	return signedBlindBlobs, nil
}
