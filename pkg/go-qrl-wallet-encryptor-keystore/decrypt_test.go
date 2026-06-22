package keystorev1_test

import (
	"encoding/json"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	keystorev1 "github.com/theQRL/qrysm/pkg/go-qrl-wallet-encryptor-keystore"
)

func TestDecrypt(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		passphrase string
		output     []byte
		err        string
	}{
		{
			name:       "NoCipher",
			input:      `{"kdf":{"function":"argon2id","params":{"dklen":32,"m":262144,"p":1,"salt":"2c2f566f38f5b79634d17267d95a0914ed47a44fe91f9cbb0b8765ebaa0b7ddd","t":8}}}`,
			passphrase: "1234567890",
			err:        "no cipher",
		},
		{
			name:       "BadSalt",
			input:      `{"kdf":{"function":"argon2id","params":{"dklen":32,"m":262144,"p":1,"salt":"z518a4d4ff18959eaef9f93d247d707945829a81c2d10983b65af6beb43d09ce","t":8}},"cipher":{"function":"aes-256-gcm","params":{"iv":"4c2275c4a14a5e984bfaec2b"},"message":"f833f12f6cb57f6961fb34bbf4ff5019c9fd70e1ab98bf0f1ba164f1b4bc773e853f973b708a4ec1b5e1148de96437ac5fc75da87c6b7293628e9d45b4bc2ab7"}}`,
			passphrase: "1234567890",
			err:        "invalid KDF salt",
		},
		{
			name:       "BadKDF",
			input:      `{"kdf":{"function":"magic","params":{"dklen":32,"m":262144,"p":1,"salt":"2c2f566f38f5b79634d17267d95a0914ed47a44fe91f9cbb0b8765ebaa0b7ddd","t":8}},"cipher":{"function":"aes-256-gcm","params":{"iv":"4c2275c4a14a5e984bfaec2b"},"message":"f833f12f6cb57f6961fb34bbf4ff5019c9fd70e1ab98bf0f1ba164f1b4bc773e853f973b708a4ec1b5e1148de96437ac5fc75da87c6b7293628e9d45b4bc2ab7"}}`,
			passphrase: "123456890",
			err:        `unsupported KDF "magic"`,
		},
		{
			name:       "BadCipherMessage",
			input:      `{"kdf":{"function":"argon2id","params":{"dklen":32,"m":262144,"p":1,"salt":"2c2f566f38f5b79634d17267d95a0914ed47a44fe91f9cbb0b8765ebaa0b7ddd","t":8}},"cipher":{"function":"aes-256-gcm","params":{"iv":"4c2275c4a14a5e984bfaec2b"},"message":"hf833f12f6cb57f6961fb34bbf4ff5019c9fd70e1ab98bf0f1ba164f1b4bc773e853f973b708a4ec1b5e1148de96437ac5fc75da87c6b7293628e9d45b4bc2ab7"}}`,
			passphrase: "1234567890",
			err:        "invalid cipher message",
		},
		{
			name:       "BadIV",
			input:      `{"kdf":{"function":"argon2id","params":{"dklen":32,"m":262144,"p":1,"salt":"2c2f566f38f5b79634d17267d95a0914ed47a44fe91f9cbb0b8765ebaa0b7ddd","t":8}},"cipher":{"function":"aes-256-gcm","params":{"iv":"h4c2275c4a14a5e984bfaec2b"},"message":"f833f12f6cb57f6961fb34bbf4ff5019c9fd70e1ab98bf0f1ba164f1b4bc773e853f973b708a4ec1b5e1148de96437ac5fc75da87c6b7293628e9d45b4bc2ab7"}}`,
			passphrase: "1234567890",
			err:        "invalid IV: encoding/hex: invalid byte: U+0068 'h'",
		},
		{
			name:       "BadCipher",
			input:      `{"kdf":{"function":"argon2id","params":{"dklen":32,"m":262144,"p":1,"salt":"2c2f566f38f5b79634d17267d95a0914ed47a44fe91f9cbb0b8765ebaa0b7ddd","t":8}},"cipher":{"function":"aes-256-ctr","params":{"iv":"4c2275c4a14a5e984bfaec2b"},"message":"f833f12f6cb57f6961fb34bbf4ff5019c9fd70e1ab98bf0f1ba164f1b4bc773e853f973b708a4ec1b5e1148de96437ac5fc75da87c6b7293628e9d45b4bc2ab7"}}`,
			passphrase: "1234567890",
			err:        `unsupported cipher "aes-256-ctr"`,
		},
		{
			name:       "Good",
			input:      `{"kdf":{"function":"argon2id","params":{"dklen":32,"m":262144,"p":1,"salt":"2c2f566f38f5b79634d17267d95a0914ed47a44fe91f9cbb0b8765ebaa0b7ddd","t":8}},"cipher":{"function":"aes-256-gcm","params":{"iv":"4c2275c4a14a5e984bfaec2b"},"message":"f833f12f6cb57f6961fb34bbf4ff5019c9fd70e1ab98bf0f1ba164f1b4bc773e853f973b708a4ec1b5e1148de96437ac5fc75da87c6b7293628e9d45b4bc2ab7"}}`,
			passphrase: "1234567890",
			output:     []byte{0x5d, 0xfd, 0xca, 0xd4, 0xf7, 0x21, 0xfe, 0x41, 0xd1, 0xbd, 0xf6, 0x32, 0xde, 0x24, 0xba, 0x60, 0xba, 0x7c, 0xfa, 0xb9, 0xc9, 0xa7, 0x92, 0x87, 0xfa, 0x0, 0x7b, 0x6a, 0xd, 0xec, 0x82, 0x0, 0xb1, 0xfa, 0x35, 0xd2, 0x57, 0x5b, 0xb1, 0x5b, 0xd4, 0x4d, 0x59, 0xb8, 0xd8, 0x78, 0x82, 0x8b},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encryptor := keystorev1.New()
			input := make(map[string]any)
			err := json.Unmarshal([]byte(test.input), &input)
			require.Nil(t, err)
			output, err := encryptor.Decrypt(input, test.passphrase)
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.Nil(t, err)
				assert.Equal(t, test.output, output)
			}
		})
	}
}

func TestDecryptBadInput(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		err   string
	}{
		{
			name: "Nil",
			err:  "no data supplied",
		},
		{
			name:  "Empty",
			input: map[string]any{},
			err:   "no cipher",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encryptor := keystorev1.New()
			_, err := encryptor.Decrypt(test.input, "irrelevant")
			require.EqualError(t, err, test.err)
		})
	}
}
func BenchmarkDecrypt(b *testing.B) {
	encryptor := keystorev1.New()
	input := make(map[string]any)
	require.NoError(b, json.Unmarshal([]byte(`{"kdf":{"function":"argon2id","params":{"dklen":32,"m":262144,"p":1,"salt":"2c2f566f38f5b79634d17267d95a0914ed47a44fe91f9cbb0b8765ebaa0b7ddd","t":8}},"cipher":{"function":"aes-256-gcm","params":{"iv":"4c2275c4a14a5e984bfaec2b"},"message":"f833f12f6cb57f6961fb34bbf4ff5019c9fd70e1ab98bf0f1ba164f1b4bc773e853f973b708a4ec1b5e1148de96437ac5fc75da87c6b7293628e9d45b4bc2ab7"}}`), &input))

	for b.Loop() {
		_, err := encryptor.Decrypt(input, "1234567890")
		require.NoError(b, err)
	}
}

func BenchmarkDecryptParallel(b *testing.B) {
	encryptor := keystorev1.New()
	input := make(map[string]any)
	require.NoError(b, json.Unmarshal([]byte(`{"kdf":{"function":"argon2id","params":{"dklen":32,"m":262144,"p":1,"salt":"2c2f566f38f5b79634d17267d95a0914ed47a44fe91f9cbb0b8765ebaa0b7ddd","t":8}},"cipher":{"function":"aes-256-gcm","params":{"iv":"4c2275c4a14a5e984bfaec2b"},"message":"f833f12f6cb57f6961fb34bbf4ff5019c9fd70e1ab98bf0f1ba164f1b4bc773e853f973b708a4ec1b5e1148de96437ac5fc75da87c6b7293628e9d45b4bc2ab7"}}`), &input))
	numCPUs := runtime.NumCPU()
	wg := &sync.WaitGroup{}

	for b.Loop() {
		wg.Add(numCPUs)
		for range numCPUs {
			go func() {
				defer wg.Done()
				_, err := encryptor.Decrypt(input, "1234567890")
				require.NoError(b, err)
			}()
		}
		wg.Wait()
	}
}
