package forkchoice

import (
	"testing"

	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/spectest/shared/common/forkchoice"
)

func TestMainnet_Altair_Forkchoice(t *testing.T) {
	forkchoice.Run(t, "mainnet", version.Altair)
}
