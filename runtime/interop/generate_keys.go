package interop

import (
	"encoding/binary"
	"sync"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/async"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	"github.com/theQRL/qrysm/v4/crypto/hash"
)

const (
	dilithiumWithdrawalPrefixByte = byte(0)
)

// DeterministicallyGenerateKeys creates Dilithium private keys.
func DeterministicallyGenerateKeys(startIndex, numKeys uint64) ([]dilithium.DilithiumKey, []dilithium.PublicKey, error) {
	dilithiumKeys := make([]dilithium.DilithiumKey, numKeys)
	pubKeys := make([]dilithium.PublicKey, numKeys)
	type keys struct {
		dilithiumKeys []dilithium.DilithiumKey
		publics       []dilithium.PublicKey
	}
	// lint:ignore uintcast -- this is safe because we can reasonably expect that the number of keys is less than max int64.
	results, err := async.Scatter(int(numKeys), func(offset int, entries int, _ *sync.RWMutex) (interface{}, error) {
		dKeys, pubs, err := deterministicallyGenerateKeys(uint64(offset)+startIndex, uint64(entries))
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

func deterministicallyGenerateKeys(startIndex, numKeys uint64) ([]dilithium.DilithiumKey, []dilithium.PublicKey, error) {
	dilithiumKeys := make([]dilithium.DilithiumKey, numKeys)
	pubKeys := make([]dilithium.PublicKey, numKeys)
	for i := startIndex; i < startIndex+numKeys; i++ {
		enc := make([]byte, 32)
		binary.LittleEndian.PutUint32(enc, uint32(i))
		// TODO: (cyyber) Hash returns 32 bytes hash, need to be replaced to get 48 bytes hash
		h := hash.Hash(enc)
		var seed [field_params.DilithiumSeedLength]uint8
		copy(seed[:], h[:])
		d, err := dilithium.SecretKeyFromSeed(seed[:])
		if err != nil {
			return nil, nil, err
		}
		dilithiumKeys[i-startIndex] = d
		pubKeys[i-startIndex] = d.PublicKey()
	}
	return dilithiumKeys, pubKeys, nil
}
