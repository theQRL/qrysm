package sanity

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/phase0/sanity"
)

func TestMainnet_Phase0_Sanity_Slots(t *testing.T) {
	sanity.RunSlotProcessingTests(t, "mainnet")
}
