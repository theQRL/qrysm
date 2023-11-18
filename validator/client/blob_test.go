package client

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func Test_validator_signBlob(t *testing.T) {
	v, m, vk, finish := setup(t)
	defer finish()

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			&zondpb.DomainRequest{
				Domain: params.BeaconConfig().DomainBlobSidecar[:],
			}). // epoch
		Return(&zondpb.DomainResponse{
			SignatureDomain: bytesutil.PadTo([]byte("signatureDomain"), 32),
		}, nil)

	blob := &zondpb.BlobSidecar{
		BlockRoot:       bytesutil.PadTo([]byte("blockRoot"), 32),
		Index:           1,
		Slot:            2,
		BlockParentRoot: bytesutil.PadTo([]byte("blockParentRoot"), 32),
		ProposerIndex:   3,
		Blob:            bytesutil.PadTo([]byte("blob"), fieldparams.BlobLength),
		KzgCommitment:   bytesutil.PadTo([]byte("kzgCommitment"), 48),
		KzgProof:        bytesutil.PadTo([]byte("kzgPRoof"), 48),
	}
	ctx := context.Background()
	sig, err := v.signBlob(ctx, blob, [dilithium2.CryptoPublicKeyBytes]byte(vk.PublicKey().Marshal()))
	require.NoError(t, err)
	pb, err := dilithium.PublicKeyFromBytes(vk.PublicKey().Marshal())
	require.NoError(t, err)
	signature, err := dilithium.SignatureFromBytes(sig)
	require.NoError(t, err)
	sr, err := signing.ComputeSigningRoot(blob, bytesutil.PadTo([]byte("signatureDomain"), 32))
	require.NoError(t, err)

	require.Equal(t, true, signature.Verify(pb, sr[:]))
}

func TestValidatorSignBlindBlob(t *testing.T) {
	// Setup
	v, m, vk, finish := setup(t)
	defer finish()

	const domainSignature = "signatureDomain"

	// Mock expectations
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // context
			&zondpb.DomainRequest{
				Domain: params.BeaconConfig().DomainBlobSidecar[:],
			}).
		Return(&zondpb.DomainResponse{
			SignatureDomain: bytesutil.PadTo([]byte(domainSignature), 32),
		}, nil)

	blobData := &zondpb.BlindedBlobSidecar{
		BlockRoot:       bytesutil.PadTo([]byte("blockRoot"), 32),
		Index:           1,
		Slot:            2,
		BlockParentRoot: bytesutil.PadTo([]byte("blockParentRoot"), 32),
		ProposerIndex:   3,
		BlobRoot:        bytesutil.PadTo([]byte("blobRoot"), 32),
		KzgCommitment:   bytesutil.PadTo([]byte("kzgCommitment"), 48),
		KzgProof:        bytesutil.PadTo([]byte("kzgProof"), 48),
	}

	ctx := context.Background()

	// Test signature creation and validation
	signatureBytes, err := v.signBlindBlob(ctx, blobData, [dilithium2.CryptoPublicKeyBytes]byte(vk.PublicKey().Marshal()))
	require.NoError(t, err)

	publicKey, err := dilithium.PublicKeyFromBytes(vk.PublicKey().Marshal())
	require.NoError(t, err)

	signature, err := dilithium.SignatureFromBytes(signatureBytes)
	require.NoError(t, err)

	signingRoot, err := signing.ComputeSigningRoot(blobData, bytesutil.PadTo([]byte(domainSignature), 32))
	require.NoError(t, err)

	// Assert that the signature is valid
	require.Equal(t, true, signature.Verify(publicKey, signingRoot[:]))
}

func Test_validator_signDenebBlobs(t *testing.T) {
	v, m, vk, finish := setup(t)
	defer finish()

	// Setting expectations for the mock client
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // context
			&zondpb.DomainRequest{
				Domain: params.BeaconConfig().DomainBlobSidecar[:],
			}).
		Return(&zondpb.DomainResponse{
			SignatureDomain: bytesutil.PadTo([]byte("signatureDomain"), 32),
		}, nil).AnyTimes() // Expecting this call multiple times since we're signing multiple blobs

	// Creating a list of Deneb blobs
	blobs := []*zondpb.BlobSidecar{
		{
			BlockRoot:       bytesutil.PadTo([]byte("blockRoot1"), 32),
			Index:           1,
			Slot:            2,
			BlockParentRoot: bytesutil.PadTo([]byte("blockParentRoot1"), 32),
			ProposerIndex:   3,
			Blob:            bytesutil.PadTo([]byte("blob1"), fieldparams.BlobLength),
			KzgCommitment:   bytesutil.PadTo([]byte("kzgCommitment1"), 48),
			KzgProof:        bytesutil.PadTo([]byte("kzgProof1"), 48),
		},
		{
			BlockRoot:       bytesutil.PadTo([]byte("blockRoot2"), 32),
			Index:           2,
			Slot:            3,
			BlockParentRoot: bytesutil.PadTo([]byte("blockParentRoot2"), 32),
			ProposerIndex:   4,
			Blob:            bytesutil.PadTo([]byte("blob2"), fieldparams.BlobLength),
			KzgCommitment:   bytesutil.PadTo([]byte("kzgCommitment2"), 48),
			KzgProof:        bytesutil.PadTo([]byte("kzgProof2"), 48),
		},
	}

	ctx := context.Background()
	signedBlobs, err := v.signDenebBlobs(ctx, blobs, [dilithium2.CryptoPublicKeyBytes]byte(vk.PublicKey().Marshal()))
	require.NoError(t, err)

	// Verify each signed blob
	for i, signedBlob := range signedBlobs {
		pb, err := dilithium.PublicKeyFromBytes(vk.PublicKey().Marshal())
		require.NoError(t, err)

		signature, err := dilithium.SignatureFromBytes(signedBlob.Signature)
		require.NoError(t, err)

		sr, err := signing.ComputeSigningRoot(blobs[i], bytesutil.PadTo([]byte("signatureDomain"), 32))
		require.NoError(t, err)

		require.Equal(t, true, signature.Verify(pb, sr[:]))
	}
}

func Test_validator_signBlindedDenebBlobs(t *testing.T) {
	v, m, vk, finish := setup(t)
	defer finish()

	// Setting expectations for the mock client
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // context
			&zondpb.DomainRequest{
				Domain: params.BeaconConfig().DomainBlobSidecar[:],
			}).
		Return(&zondpb.DomainResponse{
			SignatureDomain: bytesutil.PadTo([]byte("signatureDomain"), 32),
		}, nil).AnyTimes() // Expecting this call multiple times since we're signing multiple blobs

	// Creating a list of blinded Deneb blobs
	blindedBlobs := []*zondpb.BlindedBlobSidecar{
		{
			BlockRoot:       bytesutil.PadTo([]byte("blindedBlockRoot1"), 32),
			Index:           1,
			Slot:            2,
			BlockParentRoot: bytesutil.PadTo([]byte("blindedBlockParentRoot1"), 32),
			ProposerIndex:   3,
			BlobRoot:        bytesutil.PadTo([]byte("blobRoot1"), 32),
			KzgCommitment:   bytesutil.PadTo([]byte("kzgCommitment1"), 48),
			KzgProof:        bytesutil.PadTo([]byte("kzgProof1"), 48),
		},
		{
			BlockRoot:       bytesutil.PadTo([]byte("blindedBlockRoot2"), 32),
			Index:           2,
			Slot:            3,
			BlockParentRoot: bytesutil.PadTo([]byte("blindedBlockParentRoot2"), 32),
			ProposerIndex:   4,
			BlobRoot:        bytesutil.PadTo([]byte("blobRoot2"), 32),
			KzgCommitment:   bytesutil.PadTo([]byte("kzgCommitment2"), 48),
			KzgProof:        bytesutil.PadTo([]byte("kzgProof2"), 48),
		},
	}

	ctx := context.Background()
	signedBlindedBlobs, err := v.signBlindedDenebBlobs(ctx, blindedBlobs, [dilithium2.CryptoPublicKeyBytes]byte(vk.PublicKey().Marshal()))
	require.NoError(t, err)

	// Verify each signed blinded blob
	for i, signedBlindedBlob := range signedBlindedBlobs {
		pb, err := dilithium.PublicKeyFromBytes(vk.PublicKey().Marshal())
		require.NoError(t, err)

		signature, err := dilithium.SignatureFromBytes(signedBlindedBlob.Signature)
		require.NoError(t, err)

		sr, err := signing.ComputeSigningRoot(blindedBlobs[i], bytesutil.PadTo([]byte("signatureDomain"), 32))
		require.NoError(t, err)

		require.Equal(t, true, signature.Verify(pb, sr[:]))
	}
}
