package kv

import (
	"context"
	"fmt"
	"testing"

	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestStore_EIPBlacklistedPublicKeys(t *testing.T) {
	ctx := context.Background()
	numValidators := 100
	publicKeys := make([][field_params.MLDSA87PubkeyLength]byte, numValidators)
	for i := range numValidators {
		var key [field_params.MLDSA87PubkeyLength]byte
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
	want := make(map[[field_params.MLDSA87PubkeyLength]byte]bool)
	for _, pubKey := range publicKeys[:50] {
		want[pubKey] = true
	}
	for _, pubKey := range received {
		ok := want[pubKey]
		require.Equal(t, true, ok)
	}
}
