package types

import (
	"fmt"

	"github.com/theQRL/qrysm/v4/config/params"
)

// TODO(rgeraldes24): remove?
func StartAt(v int, c *params.BeaconChainConfig) *params.BeaconChainConfig {
	c = c.Copy()

	/*
		if v >= version.Altair {
			c.AltairForkEpoch = 0
		}
		if v >= version.Bellatrix {
			c.BellatrixForkEpoch = 0
		}
		if v >= version.Capella {
			c.CapellaForkEpoch = 0
		}

		// Time TTD to line up roughly with the bellatrix fork epoch.
		// E2E sets EL block production rate equal to SecondsPerETH1Block to keep the math simple.
		ttd := uint64(c.BellatrixForkEpoch) * uint64(c.SlotsPerEpoch) * c.SecondsPerSlot
	*/

	c.AltairForkEpoch = 0
	c.BellatrixForkEpoch = 0
	c.CapellaForkEpoch = 0
	c.TerminalTotalDifficulty = fmt.Sprintf("%d", 0)
	return c
}
