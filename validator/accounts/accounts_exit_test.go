package accounts

import (
	"testing"

	"github.com/cyyber/qrysm/v4/encoding/bytesutil"
	"github.com/cyyber/qrysm/v4/testing/assert"
	"github.com/cyyber/qrysm/v4/testing/require"
	"github.com/sirupsen/logrus/hooks/test"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
)

func TestDisplayExitInfo(t *testing.T) {
	logHook := test.NewGlobal()
	key := []byte("0x123456")
	displayExitInfo([][]byte{key}, []string{string(key)})
	assert.LogsContain(t, logHook, "https://beaconcha.in/validator/3078313233343536")
}

func TestDisplayExitInfo_NoKeys(t *testing.T) {
	logHook := test.NewGlobal()
	displayExitInfo([][]byte{}, []string{})
	assert.LogsContain(t, logHook, "No successful voluntary exits")
}

func TestPrepareAllKeys(t *testing.T) {
	key1 := bytesutil.ToBytes2592([]byte("key1"))
	key2 := bytesutil.ToBytes2592([]byte("key2"))
	raw, formatted := prepareAllKeys([][dilithium2.CryptoPublicKeyBytes]byte{key1, key2})
	require.Equal(t, 2, len(raw))
	require.Equal(t, 2, len(formatted))
	assert.DeepEqual(t, bytesutil.ToBytes2592([]byte{107, 101, 121, 49}), bytesutil.ToBytes2592(raw[0]))
	assert.DeepEqual(t, bytesutil.ToBytes2592([]byte{107, 101, 121, 50}), bytesutil.ToBytes2592(raw[1]))
	assert.Equal(t, "0x6b6579310000", formatted[0])
	assert.Equal(t, "0x6b6579320000", formatted[1])
}
