package local

import (
	"context"
	"encoding/hex"
	"testing"

	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestLocalKeymanager_ExtractKeystores(t *testing.T) {
	mlDSA87KeysCache = make(map[[field_params.MLDSA87PubkeyLength]byte]ml_dsa_87.MLDSA87Key)
	dr := &Keymanager{}
	validatingKeys := make([]ml_dsa_87.MLDSA87Key, 10)
	for i := range validatingKeys {
		secretKey, err := ml_dsa_87.RandKey()
		require.NoError(t, err)
		validatingKeys[i] = secretKey
		mlDSA87KeysCache[bytesutil.ToBytes2592(secretKey.PublicKey().Marshal())] = secretKey
	}
	ctx := context.Background()
	password := "password"

	// Extracting 0 public keys should return 0 keystores.
	keystores, err := dr.ExtractKeystores(ctx, nil, password)
	require.NoError(t, err)
	assert.Equal(t, 0, len(keystores))

	// We attempt to extract a few indices.
	keystores, err = dr.ExtractKeystores(
		ctx,
		[]ml_dsa_87.PublicKey{
			validatingKeys[3].PublicKey(),
			validatingKeys[5].PublicKey(),
			validatingKeys[7].PublicKey(),
		},
		password,
	)
	require.NoError(t, err)
	receivedPubKeys := make([][]byte, len(keystores))
	for i, k := range keystores {
		pubKeyBytes, err := hex.DecodeString(k.Pubkey)
		require.NoError(t, err)
		receivedPubKeys[i] = pubKeyBytes
	}
	assert.DeepEqual(t, receivedPubKeys, [][]byte{
		validatingKeys[3].PublicKey().Marshal(),
		validatingKeys[5].PublicKey().Marshal(),
		validatingKeys[7].PublicKey().Marshal(),
	})
}
