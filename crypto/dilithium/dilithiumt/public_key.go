package dilithiumt

import (
	"fmt"
	"reflect"

	lruwrpr "github.com/cyyber/qrysm/v4/cache/lru"
	"github.com/cyyber/qrysm/v4/crypto/bls/common"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
)

var maxKeys = 1000000
var pubkeyCache = lruwrpr.New(maxKeys)

type PublicKey struct {
	p *[dilithium2.CryptoPublicKeyBytes]uint8
}

func (p *PublicKey) Marshal() []byte {
	return p.p[:]
}

func PublicKeyFromBytes(pubKey []byte) (common.PublicKey, error) {
	if len(pubKey) != dilithium2.CryptoPublicKeyBytes {
		return nil, fmt.Errorf("public key must be %d bytes", dilithium2.CryptoPublicKeyBytes)
	}
	newKey := (*[dilithium2.CryptoPublicKeyBytes]uint8)(pubKey)
	if cv, ok := pubkeyCache.Get(*newKey); ok {
		return cv.(*PublicKey).Copy(), nil
	}
	var p [dilithium2.CryptoPublicKeyBytes]uint8
	copy(p[:], pubKey)
	pubKeyObj := &PublicKey{p: &p}
	copiedKey := pubKeyObj.Copy()
	cacheKey := *newKey
	pubkeyCache.Add(cacheKey, copiedKey)
	return pubKeyObj, nil
}

func AggregatePublicKeys(pubs [][]byte) (common.PublicKey, error) {
	panic("AggregatePublicKeys not supported for dilithium")
}

func (p *PublicKey) Copy() common.PublicKey {
	np := *p.p
	return &PublicKey{p: &np}
}

func (p *PublicKey) IsInfinite() bool {
	var zeroKey [dilithium2.CryptoPublicKeyBytes]uint8
	return reflect.DeepEqual(p.p, zeroKey)
}

func (p *PublicKey) Equals(p2 common.PublicKey) bool {
	return reflect.DeepEqual(p.p, p2.(*PublicKey).p)
}

func (p *PublicKey) Aggregate(p2 common.PublicKey) common.PublicKey {
	panic("Aggregate not supported for dilithium")
}

func AggregateMultiplePubkeys(pubkeys []common.PublicKey) common.PublicKey {
	panic("AggregateMultiplePubkeys not supported for dilithium")
}
