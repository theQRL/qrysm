//go:build !noMainnetGenesis
// +build !noMainnetGenesis

package genesis

import (
	_ "embed"

	"github.com/theQRL/qrysm/v4/config/params"
)

var (
	// TODO(theQRL/qrysm/issues/81)
	//go:embed mainnet.ssz.snappy
	mainnetRawSSZCompressed []byte
)

func init() {
	embeddedStates[params.MainnetName] = &mainnetRawSSZCompressed
}
