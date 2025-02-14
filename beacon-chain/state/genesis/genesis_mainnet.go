//go:build !noMainnetGenesis
// +build !noMainnetGenesis

package genesis

import (
	_ "embed"

	"github.com/theQRL/qrysm/config/params"
)

var (
	// TODO(now.youtrack.cloud/issue/TQ-11)
	//go:embed mainnet.ssz.snappy
	mainnetRawSSZCompressed []byte
)

func init() {
	embeddedStates[params.MainnetName] = &mainnetRawSSZCompressed
}
