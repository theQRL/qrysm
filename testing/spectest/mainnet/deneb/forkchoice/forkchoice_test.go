package forkchoice

import (
	"testing"

	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/spectest/shared/common/forkchoice"
)

func TestMainnet_Deneb_Forkchoice(t *testing.T) {
	forkchoice.Run(t, "mainnet", version.Deneb)
}
