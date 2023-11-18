package finality

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/deneb/finality"
)

func TestMainnet_Deneb_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "mainnet")
}
