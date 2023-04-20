package interop

import (
	"encoding/binary"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v4/async"
	"github.com/prysmaticlabs/prysm/v4/crypto/dilithium"
	"github.com/prysmaticlabs/prysm/v4/crypto/hash"
	"github.com/theQRL/go-qrllib/common"
	"sync"
)

func QDeterministicallyGenerateKeys(startIndex, numKeys uint64) ([]dilithium.DilithiumKey, []dilithium.PublicKey, error) {
	dilithiumKeys := make([]dilithium.DilithiumKey, numKeys)
	pubKeys := make([]dilithium.PublicKey, numKeys)
	type keys struct {
		dilithiumKeys []dilithium.DilithiumKey
		publics       []dilithium.PublicKey
	}
	// lint:ignore uintcast -- this is safe because we can reasonably expect that the number of keys is less than max int64.
	results, err := async.Scatter(int(numKeys), func(offset int, entries int, _ *sync.RWMutex) (interface{}, error) {
		dKeys, pubs, err := qdeterministicallyGenerateKeys(uint64(offset)+startIndex, uint64(entries))
		return &keys{dilithiumKeys: dKeys, publics: pubs}, err
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to generate keys")
	}
	for _, result := range results {
		if keysExtent, ok := result.Extent.(*keys); ok {
			copy(dilithiumKeys[result.Offset:], keysExtent.dilithiumKeys)
			copy(pubKeys[result.Offset:], keysExtent.publics)
		} else {
			return nil, nil, errors.New("extent not of expected type")
		}
	}
	return dilithiumKeys, pubKeys, nil
}

func qdeterministicallyGenerateKeys(startIndex, numKeys uint64) ([]dilithium.DilithiumKey, []dilithium.PublicKey, error) {
	dilithiumKeys := make([]dilithium.DilithiumKey, numKeys)
	pubKeys := make([]dilithium.PublicKey, numKeys)
	for i := startIndex; i < startIndex+numKeys; i++ {
		enc := make([]byte, 32)
		binary.LittleEndian.PutUint32(enc, uint32(i))
		// TODO: (cyyber) Hash returns 32 bytes hash, need to be replaced to get 48 bytes hash
		h := hash.Hash(enc)
		var seed [common.SeedSize]uint8
		copy(seed[:], h[:])
		d, err := dilithium.SecretKeyFromBytes(seed[:])
		if err != nil {
			return nil, nil, err
		}
		dilithiumKeys[i-startIndex] = d
		pubKeys[i-startIndex] = d.PublicKey()
	}
	return dilithiumKeys, pubKeys, nil
}
