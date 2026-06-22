package interop

import (
	"encoding/binary"
	"sync"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/async"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/crypto/hash"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
)

// DeterministicallyGenerateKeys creates ML-DSA-87 private keys.
func DeterministicallyGenerateKeys(startIndex, numKeys uint64) ([]ml_dsa_87.MLDSA87Key, []ml_dsa_87.PublicKey, error) {
	mlDSA87Keys := make([]ml_dsa_87.MLDSA87Key, numKeys)
	pubKeys := make([]ml_dsa_87.PublicKey, numKeys)
	type keys struct {
		mlDSA87Keys []ml_dsa_87.MLDSA87Key
		publics     []ml_dsa_87.PublicKey
	}
	// lint:ignore uintcast -- this is safe because we can reasonably expect that the number of keys is less than max int64.
	results, err := async.Scatter(int(numKeys), func(offset int, entries int, _ *sync.RWMutex) (any, error) {
		dKeys, pubs, err := deterministicallyGenerateKeys(uint64(offset)+startIndex, uint64(entries))
		return &keys{mlDSA87Keys: dKeys, publics: pubs}, err
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to generate keys")
	}
	for _, result := range results {
		if keysExtent, ok := result.Extent.(*keys); ok {
			copy(mlDSA87Keys[result.Offset:], keysExtent.mlDSA87Keys)
			copy(pubKeys[result.Offset:], keysExtent.publics)
		} else {
			return nil, nil, errors.New("extent not of expected type")
		}
	}
	return mlDSA87Keys, pubKeys, nil
}

func deterministicallyGenerateKeys(startIndex, numKeys uint64) ([]ml_dsa_87.MLDSA87Key, []ml_dsa_87.PublicKey, error) {
	mlDSA87Keys := make([]ml_dsa_87.MLDSA87Key, numKeys)
	pubKeys := make([]ml_dsa_87.PublicKey, numKeys)
	for i := startIndex; i < startIndex+numKeys; i++ {
		enc := make([]byte, 32)
		binary.LittleEndian.PutUint32(enc, uint32(i))
		// TODO: (cyyber) Hash returns 32 bytes hash, need to be replaced to get 48 bytes hash
		h := hash.Hash(enc)
		var seed [fieldparams.MLDSA87SeedLength]uint8
		copy(seed[:], h[:])
		d, err := ml_dsa_87.SecretKeyFromSeed(seed[:])
		if err != nil {
			return nil, nil, err
		}
		mlDSA87Keys[i-startIndex] = d
		pubKeys[i-startIndex] = d.PublicKey()
	}
	return mlDSA87Keys, pubKeys, nil
}
