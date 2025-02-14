package dilithiumt

import (
	"fmt"

	"github.com/theQRL/go-qrllib/dilithium"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/crypto/dilithium/common"
	"github.com/theQRL/qrysm/crypto/rand"
)

type dilithiumKey struct {
	d *dilithium.Dilithium
}

func RandKey() (common.SecretKey, error) {
	var seed [field_params.DilithiumSeedLength]uint8
	_, err := rand.NewGenerator().Read(seed[:])
	if err != nil {
		return nil, err
	}
	d, err := dilithium.NewDilithiumFromSeed(seed)
	if err != nil {
		return nil, err
	}
	return &dilithiumKey{d: d}, nil
}

func SecretKeyFromSeed(seed []byte) (common.SecretKey, error) {
	if len(seed) != field_params.DilithiumSeedLength {
		return nil, fmt.Errorf("secret key must be %d bytes", field_params.DilithiumSeedLength)
	}
	var sizedSeed [field_params.DilithiumSeedLength]uint8
	copy(sizedSeed[:], seed)

	d, err := dilithium.NewDilithiumFromSeed(sizedSeed)
	if err != nil {
		return nil, err
	}
	return &dilithiumKey{d: d}, nil
}

// PublicKey obtains the public key corresponding to the Dilithium secret key.
func (d *dilithiumKey) PublicKey() common.PublicKey {
	p := d.d.GetPK()
	return &PublicKey{p: &p}
}

func (d *dilithiumKey) Sign(msg []byte) common.Signature {
	signature, err := d.d.Sign(msg)
	if err != nil {
		return nil
	}
	return &Signature{s: &signature}
}

// Marshal a secret key into a LittleEndian byte slice.
func (d *dilithiumKey) Marshal() []byte {
	keyBytes := d.d.GetSeed()
	return keyBytes[:]
}
