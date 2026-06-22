package keystorev1_test

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	keystorev1 "github.com/theQRL/qrysm/pkg/go-qrl-wallet-encryptor-keystore"
	qrltypes "github.com/theQRL/qrysm/pkg/go-qrl-wallet-types"
)

func TestInterfaces(t *testing.T) {
	encryptor := keystorev1.New()
	require.Implements(t, (*qrltypes.Encryptor)(nil), encryptor)
}

func TestRoundTrip(t *testing.T) {
	secret, err := hex.DecodeString("5dfdcad4f721fe41d1bdf632de24ba60ba7cfab9c9a79287fa007b6a0dec8200b1fa35d2575bb15bd44d59b8d878828b")
	require.NoError(t, err)

	tests := []struct {
		name       string
		input      string
		passphrase string
		secret     []byte
		options    []keystorev1.Option
		err        error
	}{
		{
			name:       "Test1",
			input:      `{"kdf":{"function":"argon2id","params":{"dklen":32,"m":262144,"p":1,"salt":"2c2f566f38f5b79634d17267d95a0914ed47a44fe91f9cbb0b8765ebaa0b7ddd","t":8}},"cipher":{"function":"aes-256-gcm","params":{"iv":"4c2275c4a14a5e984bfaec2b"},"message":"f833f12f6cb57f6961fb34bbf4ff5019c9fd70e1ab98bf0f1ba164f1b4bc773e853f973b708a4ec1b5e1148de96437ac5fc75da87c6b7293628e9d45b4bc2ab7"}}`,
			passphrase: "1234567890",
			secret:     secret,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encryptor := keystorev1.New(test.options...)
			input := make(map[string]any)
			err := json.Unmarshal([]byte(test.input), &input)
			require.Nil(t, err)
			secret, err := encryptor.Decrypt(input, test.passphrase)
			if test.err != nil {
				require.NotNil(t, err)
				assert.Equal(t, test.err.Error(), err.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, test.secret, secret)
				newInput, err := encryptor.Encrypt(secret, test.passphrase)
				require.Nil(t, err)
				newSecret, err := encryptor.Decrypt(newInput, test.passphrase)
				require.Nil(t, err)
				require.Equal(t, test.secret, newSecret)
			}
		})
	}
}

func TestNameAndVersion(t *testing.T) {
	encryptor := keystorev1.New()
	assert.Equal(t, "keystore", encryptor.Name())
	assert.Equal(t, uint(1), encryptor.Version())
	assert.Equal(t, "keystorev1", encryptor.String())
}

func TestGenerateKey(t *testing.T) {
	encryptor := keystorev1.New()
	x, err := encryptor.Encrypt([]byte{0xaa, 0xff, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x12, 0x23, 0x45, 0x67, 0x78, 0xe9, 0x42, 0x61, 0x71, 0x9d, 0x3d, 0x4d, 0x5e, 0xff, 0xfc, 0xcc, 0xae, 0xea, 0x82, 0x21, 0x05, 0x01, 0x74, 0x32}, "")
	require.Nil(t, err)
	assert.NotNil(t, x)
}
