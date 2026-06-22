package keystorev1

import "fmt"

// Encryptor is an encryptor that follows the QRL keystore V1 specification.
type Encryptor struct {
	cipher string
}

type ksKDFParams struct {
	// Shared parameters
	Salt  string `json:"salt"`
	DKLen int    `json:"dklen"`
	// Argon2id-specific parameters
	T int `json:"t,omitempty"`
	M int `json:"m,omitempty"`
	P int `json:"p,omitempty"`
}

type ksKDF struct {
	Function string       `json:"function"`
	Params   *ksKDFParams `json:"params"`
}

type ksCipherParams struct {
	// AES-256-GCM-specific parameters
	IV string `json:"iv,omitempty"`
}

type ksCipher struct {
	Function string          `json:"function"`
	Params   *ksCipherParams `json:"params"`
	Message  string          `json:"message"`
}

type keystoreV1 struct {
	KDF    *ksKDF    `json:"kdf"`
	Cipher *ksCipher `json:"cipher"`
}

const (
	name    = "keystore"
	version = 1
)

// options are the options for the keystore encryptor.
type options struct {
	cipher string
}

// Option gives options to New.
type Option interface {
	apply(opts *options)
}

type optionFunc func(*options)

func (f optionFunc) apply(o *options) {
	f(o)
}

// WithCipher sets the cipher for the encryptor.
func WithCipher(cipher string) Option {
	return optionFunc(func(o *options) {
		o.cipher = cipher
	})
}

// New creates a new keystore V1 encryptor.
// This takes the following options:
// - cipher: the cipher to use when encrypting the secret, can be "argon2id" (default).
func New(opts ...Option) *Encryptor {
	options := options{
		cipher: algoArgon2id,
	}
	for _, o := range opts {
		o.apply(&options)
	}

	return &Encryptor{
		cipher: options.cipher,
	}
}

// Name returns the name of this encryptor.
func (e *Encryptor) Name() string {
	return name
}

// Version returns the version of this encryptor.
func (e *Encryptor) Version() uint {
	return version
}

// String returns a string representing this encryptor.
func (e *Encryptor) String() string {
	return fmt.Sprintf("%sv%d", name, version)
}
