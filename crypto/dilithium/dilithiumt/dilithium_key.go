package dilithiumt

import (
	"fmt"

	common2 "github.com/theQRL/go-qrllib/common"
	"github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/crypto/bls/common"
	"github.com/theQRL/qrysm/v4/crypto/rand"
)

type dilithiumKey struct {
	d *dilithium.Dilithium
}

func RandKey() (common.SecretKey, error) {
	var seed [common2.SeedSize]uint8
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

func SecretKeyFromBytes(seed []byte) (common.SecretKey, error) {
	if len(seed) != common2.SeedSize {
		return nil, fmt.Errorf("secret key must be %d bytes", common2.SeedSize)
	}
	var sizedSeed [common2.SeedSize]uint8
	copy(sizedSeed[:], seed)

	d, err := dilithium.NewDilithiumFromSeed(sizedSeed)
	if err != nil {
		return nil, err
	}
	return &dilithiumKey{d: d}, nil
}

// PublicKey obtains the public key corresponding to the BLS secret key.
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
