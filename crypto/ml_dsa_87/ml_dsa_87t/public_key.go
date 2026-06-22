package ml_dsa_87t

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/theQRL/go-qrllib/wallet/ml_dsa_87"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87/common"
)

var pubkeyCache = &pubkeyCacheMap{
	items: make(map[[field_params.MLDSA87PubkeyLength]byte]common.PublicKey),
}

// pubkeyCacheMap caches uncompressed Dilithium public keys keyed by their compressed bytes.
// A plain map+mutex is used instead of an LRU because the active validator set isn't bounded
// in a way that makes LRU eviction useful — eviction just forces the expensive uncompression
// to repeat under churn. Memory grows with the number of distinct public keys ever seen by
// the process, which is acceptable given each entry is a small fixed-size struct.
type pubkeyCacheMap struct {
	mu    sync.RWMutex
	items map[[field_params.MLDSA87PubkeyLength]byte]common.PublicKey
}

func (c *pubkeyCacheMap) pubkey(key [field_params.MLDSA87PubkeyLength]byte) (common.PublicKey, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	v, ok := c.items[key]
	return v, ok
}

func (c *pubkeyCacheMap) setPubkey(key [field_params.MLDSA87PubkeyLength]byte, value common.PublicKey) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = value
}

type PublicKey struct {
	p *ml_dsa_87.PK
}

func (p *PublicKey) Marshal() []byte {
	return p.p[:]
}

func PublicKeyFromBytes(pubKey []byte) (common.PublicKey, error) {
	return publicKeyFromBytes(pubKey, true)
}

func publicKeyFromBytes(pubKey []byte, cacheCopy bool) (common.PublicKey, error) {
	if len(pubKey) != field_params.MLDSA87PubkeyLength {
		return nil, fmt.Errorf("public key must be %d bytes", field_params.MLDSA87PubkeyLength)
	}
	newKey := (*[field_params.MLDSA87PubkeyLength]uint8)(pubKey)
	if cv, ok := pubkeyCache.pubkey(*newKey); ok {
		if cacheCopy {
			return cv.(*PublicKey).Copy(), nil
		}
		return cv.(*PublicKey), nil
	}
	var p ml_dsa_87.PK
	copy(p[:], pubKey)
	pubKeyObj := &PublicKey{p: &p}
	copiedKey := pubKeyObj.Copy()
	pubkeyCache.setPubkey(*newKey, copiedKey)
	return pubKeyObj, nil
}

func (p *PublicKey) Copy() common.PublicKey {
	np := *p.p
	return &PublicKey{p: &np}
}

func (p *PublicKey) Equals(p2 common.PublicKey) bool {
	return reflect.DeepEqual(p.p, p2.(*PublicKey).p)
}
