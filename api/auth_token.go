package api

import (
	"crypto/rand"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common/hexutil"
)

const AuthTokenFileName = "auth-token"

// GenerateRandomHexString returns a 32-byte auth token encoded as 0x-prefixed hex.
func GenerateRandomHexString() (string, error) {
	secret := make([]byte, 32)
	n, err := rand.Read(secret)
	if err != nil {
		return "", err
	}
	if n != len(secret) {
		return "", errors.New("rand: unexpected length")
	}
	return hexutil.Encode(secret), nil
}

// ValidateAuthToken ensures auth tokens are 0x-prefixed hex strings with at least 32 bytes.
func ValidateAuthToken(token string) error {
	b, err := hexutil.Decode(token)
	if err != nil || len(b) < 32 {
		return errors.New("invalid auth token: token should be hex-encoded and at least 256 bits")
	}
	return nil
}
