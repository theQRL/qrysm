package kv

import (
	"context"
	"fmt"
	"testing"

	"github.com/cyyber/qrysm/v4/testing/assert"
	"github.com/cyyber/qrysm/v4/testing/require"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
)

func TestStore_EIPBlacklistedPublicKeys(t *testing.T) {
	ctx := context.Background()
	numValidators := 100
	publicKeys := make([][dilithium2.CryptoPublicKeyBytes]byte, numValidators)
	for i := 0; i < numValidators; i++ {
		var key [dilithium2.CryptoPublicKeyBytes]byte
		copy(key[:], fmt.Sprintf("%d", i))
		publicKeys[i] = key
	}

	// No blacklisted keys returns empty.
	validatorDB := setupDB(t, publicKeys)
	received, err := validatorDB.EIPImportBlacklistedPublicKeys(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, len(received))

	// Save half of the public keys as as blacklisted and attempt to retrieve.
	err = validatorDB.SaveEIPImportBlacklistedPublicKeys(ctx, publicKeys[:50])
	require.NoError(t, err)
	received, err = validatorDB.EIPImportBlacklistedPublicKeys(ctx)
	require.NoError(t, err)

	// Keys are not guaranteed to be ordered, so we create a map for comparisons.
	want := make(map[[dilithium2.CryptoPublicKeyBytes]byte]bool)
	for _, pubKey := range publicKeys[:50] {
		want[pubKey] = true
	}
	for _, pubKey := range received {
		ok := want[pubKey]
		require.Equal(t, true, ok)
	}
}
