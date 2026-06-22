package types

type Encryptor interface {
	Name() string

	Version() uint

	Encrypt(data []byte, key string) (map[string]any, error)

	Decrypt(data map[string]any, key string) ([]byte, error)
}
