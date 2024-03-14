package dilithiumt

import (
	"fmt"

	"github.com/theQRL/qrysm/v4/cache/nonblocking"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/crypto/dilithium/common"
)

func init() {
	onEvict := func(_ [field_params.DilithiumPubkeyLength]byte, _ common.PublicKey) {}
	keysCache, err := nonblocking.NewLRU(maxKeys, onEvict)
	if err != nil {
		panic(fmt.Sprintf("Could not initiate public keys cache: %v", err))
	}
	pubkeyCache = keysCache
}
