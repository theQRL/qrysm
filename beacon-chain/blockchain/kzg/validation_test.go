package kzg

import (
	"testing"

	GoKZG "github.com/crate-crypto/go-kzg-4844"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestIsDataAvailable(t *testing.T) {
	sidecars := make([]*zondpb.BlobSidecar, 0)
	commitments := make([][]byte, 0)
	require.NoError(t, IsDataAvailable(commitments, sidecars))
}

func TestBytesToAny(t *testing.T) {
	bytes := []byte{0x01, 0x02}
	blob := GoKZG.Blob{0x01, 0x02}
	commitment := GoKZG.KZGCommitment{0x01, 0x02}
	proof := GoKZG.KZGProof{0x01, 0x02}
	require.DeepEqual(t, blob, bytesToBlob(bytes))
	require.DeepEqual(t, commitment, bytesToCommitment(bytes))
	require.DeepEqual(t, proof, bytesToKZGProof(bytes))
}
