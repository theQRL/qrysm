package local

import (
	"context"
	"runtime"
	"sync"

	"github.com/pkg/errors"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/crypto/randao"
)

var (
	onionsLock sync.Mutex
	// onions caches the RANDAO hash onion of every key in the keys cache. An
	// onion costs randao.DefaultLayers hashes to build and ~32 KiB to keep.
	onions = make(map[[field_params.MLDSA87PubkeyLength]byte]*randao.Onion)
)

// RandaoReveal returns the pre-image of commitment in the validator's RANDAO
// hash onion, deriving the onion from the validator's ML-DSA-87 seed on first
// use.
func (*Keymanager) RandaoReveal(
	_ context.Context,
	publicKey [field_params.MLDSA87PubkeyLength]byte,
	commitment [field_params.RandaoCommitmentLength]byte,
) ([field_params.RandaoRevealLength]byte, error) {
	o, err := onionFor(publicKey)
	if err != nil {
		return [field_params.RandaoRevealLength]byte{}, err
	}
	reveal, err := o.Reveal(commitment)
	if err != nil {
		return [field_params.RandaoRevealLength]byte{}, errors.Wrap(err, "could not derive randao reveal")
	}
	return reveal, nil
}

// onionFor returns the cached onion for publicKey, building it if needed.
func onionFor(publicKey [field_params.MLDSA87PubkeyLength]byte) (*randao.Onion, error) {
	onionsLock.Lock()
	o, ok := onions[publicKey]
	onionsLock.Unlock()
	if ok {
		return o, nil
	}
	lock.RLock()
	secretKey, ok := mlDSA87KeysCache[publicKey]
	lock.RUnlock()
	if !ok {
		return nil, errors.New("no signing key found in keys cache")
	}
	o, err := randao.NewOnion(secretKey.Marshal(), randao.DefaultLayers)
	if err != nil {
		return nil, err
	}
	onionsLock.Lock()
	if existing, ok := onions[publicKey]; ok {
		o = existing // another goroutine won the race; keep its position hint
	} else {
		onions[publicKey] = o
	}
	onionsLock.Unlock()
	return o, nil
}

// warmRandaoOnions builds the onions of the given keys in the background,
// using every core. Keys removed from the cache in the meantime are skipped.
func warmRandaoOnions(publicKeys [][field_params.MLDSA87PubkeyLength]byte) {
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for _, pk := range publicKeys {
		wg.Add(1)
		sem <- struct{}{}
		go func(pk [field_params.MLDSA87PubkeyLength]byte) {
			defer wg.Done()
			defer func() { <-sem }()
			if _, err := onionFor(pk); err != nil {
				log.WithError(err).Debug("Could not build randao onion")
			}
		}(pk)
	}
	wg.Wait()
}

// resetRandaoOnions drops every cached onion.
func resetRandaoOnions() {
	onionsLock.Lock()
	onions = make(map[[field_params.MLDSA87PubkeyLength]byte]*randao.Onion)
	onionsLock.Unlock()
}
