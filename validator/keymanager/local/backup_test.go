package local

import (
	"context"
	"encoding/hex"
	"testing"

	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/crypto/bls"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestLocalKeymanager_ExtractKeystores(t *testing.T) {
	dilithiumKeysCache = make(map[[dilithium2.CryptoPublicKeyBytes]byte]dilithium.DilithiumKey)
	dr := &Keymanager{}
	validatingKeys := make([]bls.SecretKey, 10)
	for i := 0; i < len(validatingKeys); i++ {
		secretKey, err := bls.RandKey()
		require.NoError(t, err)
		validatingKeys[i] = secretKey
		dilithiumKeysCache[bytesutil.ToBytes2592(secretKey.PublicKey().Marshal())] = secretKey
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
		[]bls.PublicKey{
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
