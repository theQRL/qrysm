package finality

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/bellatrix/finality"
)

func TestMainnet_Bellatrix_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "mainnet")
}
