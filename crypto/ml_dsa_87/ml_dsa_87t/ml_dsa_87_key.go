package ml_dsa_87t

import (
	"fmt"

	"github.com/theQRL/go-qrllib/wallet/ml_dsa_87"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87/common"
	"github.com/theQRL/qrysm/crypto/rand"
)

type mlDSA87Key struct {
	w *ml_dsa_87.Wallet
}

func RandKey() (common.SecretKey, error) {
	var seed [field_params.MLDSA87SeedLength]uint8
	_, err := rand.NewGenerator().Read(seed[:])
	if err != nil {
		return nil, err
	}
	w, err := ml_dsa_87.NewWalletFromSeed(seed)
	if err != nil {
		return nil, err
	}
	return &mlDSA87Key{w: w}, nil
}

func SecretKeyFromSeed(seed []byte) (common.SecretKey, error) {
	if len(seed) != field_params.MLDSA87SeedLength {
		return nil, fmt.Errorf("secret key must be %d bytes", field_params.MLDSA87SeedLength)
	}
	var sizedSeed [field_params.MLDSA87SeedLength]uint8
	copy(sizedSeed[:], seed)

	w, err := ml_dsa_87.NewWalletFromSeed(sizedSeed)
	if err != nil {
		return nil, err
	}
	return &mlDSA87Key{w: w}, nil
}

// PublicKey obtains the public key corresponding to the ML-DSA-87 secret key.
func (m *mlDSA87Key) PublicKey() common.PublicKey {
	p := m.w.GetPK()
	return &PublicKey{p: &p}
}

// Sign signs the message with the hedged ML-DSA-87 signing scheme. The
// underlying wallet draws randomness for every signature, so signing can fail
// (e.g. on entropy exhaustion); the error must be propagated instead of
// returning a nil signature that callers would dereference.
func (m *mlDSA87Key) Sign(msg []byte) (common.Signature, error) {
	signature, err := m.w.Sign(msg)
	if err != nil {
		return nil, fmt.Errorf("could not sign message: %w", err)
	}
	return &Signature{s: &signature}, nil
}

func (m *mlDSA87Key) SignDeterministic(msg []byte) (common.Signature, error) {
	signature, err := m.w.SignDeterministic(msg)
	if err != nil {
		return nil, err
	}
	return &Signature{s: &signature}, nil
}

// Marshal a secret key into a LittleEndian byte slice.
func (m *mlDSA87Key) Marshal() []byte {
	keyBytes := m.w.GetSeed()
	return keyBytes[:]
}
