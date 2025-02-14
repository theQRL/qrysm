package keystore

import (
	"bytes"
	"os"
	"path"
	"testing"

	"github.com/pborman/uuid"
	"github.com/theQRL/qrysm/crypto/dilithium"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/require"
)

func TestMarshalAndUnmarshal(t *testing.T) {
	testID := uuid.NewRandom()
	dilithiumKey, err := dilithium.RandKey()
	require.NoError(t, err)

	key := &Key{
		ID:        testID,
		SecretKey: dilithiumKey,
		PublicKey: dilithiumKey.PublicKey(),
	}
	marshalledObject, err := key.MarshalJSON()
	require.NoError(t, err)
	newKey := &Key{
		ID:        []byte{},
		SecretKey: dilithiumKey,
		PublicKey: dilithiumKey.PublicKey(),
	}

	err = newKey.UnmarshalJSON(marshalledObject)
	require.NoError(t, err)
	require.Equal(t, true, bytes.Equal(newKey.ID, testID))
}

func TestStoreRandomKey(t *testing.T) {
	ks := &Keystore{
		keysDirPath: path.Join(t.TempDir(), "keystore"),
		scryptN:     LightScryptN,
		scryptP:     LightScryptP,
	}
	require.NoError(t, storeNewRandomKey(ks, "password"))
}

func TestNewKeyFromDilithium(t *testing.T) {
	b := []byte("hi")
	b48 := bytesutil.ToBytes48(b)
	dilithiumkey, err := dilithium.SecretKeyFromSeed(b48[:])
	require.NoError(t, err)
	key, err := NewKeyFromDilithium(dilithiumkey)
	require.NoError(t, err)

	expected := dilithiumkey.Marshal()
	require.Equal(t, true, bytes.Equal(expected, key.SecretKey.Marshal()))
	_, err = NewKey()
	require.NoError(t, err)
}

func TestWriteFile(t *testing.T) {
	tempDir := path.Join(t.TempDir(), "keystore", "file")
	testKeystore := []byte{'t', 'e', 's', 't'}

	err := writeKeyFile(tempDir, testKeystore)
	require.NoError(t, err)

	keystore, err := os.ReadFile(tempDir)
	require.NoError(t, err)
	require.Equal(t, true, bytes.Equal(keystore, testKeystore))
}
