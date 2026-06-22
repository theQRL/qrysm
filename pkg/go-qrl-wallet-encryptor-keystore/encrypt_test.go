package keystorev1_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	keystorev1 "github.com/theQRL/qrysm/pkg/go-qrl-wallet-encryptor-keystore"
)

func TestEncrypt(t *testing.T) {
	tests := []struct {
		name       string
		cipher     string
		secret     []byte
		passphrase string
		err        error
	}{
		{
			name:       "Nil",
			cipher:     "argon2id",
			secret:     nil,
			passphrase: "",
			err:        errors.New("no secret"),
		},
		{
			name:       "EmptyArgon2id",
			cipher:     "argon2id",
			secret:     []byte(""),
			passphrase: "",
		},
		{
			name:       "EmptyArgon2id2",
			secret:     []byte(""),
			passphrase: "",
		},
		{
			name:       "UnknownCipher",
			cipher:     "unknown",
			secret:     []byte(""),
			passphrase: "",
			err:        errors.New(`unknown cipher "unknown"`),
		},
		{
			name:   "Good",
			cipher: "argon2id",
			secret: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
				0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
				0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f,
				0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f,
			},
			passphrase: "wallet passphrase",
		},
		{
			name:       "LargeInput",
			cipher:     "argon2id",
			secret:     []byte("................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................"),
			passphrase: "test",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var options []keystorev1.Option
			if test.cipher != "" {
				options = append(options, keystorev1.WithCipher(test.cipher))
			}
			encryptor := keystorev1.New(options...)
			_, err := encryptor.Encrypt(test.secret, test.passphrase)
			if test.err != nil {
				require.NotNil(t, err)
				assert.Equal(t, test.err.Error(), err.Error())
			} else {
				require.Nil(t, err)
			}
		})
	}
}
