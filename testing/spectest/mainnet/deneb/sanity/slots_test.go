package sanity

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/deneb/sanity"
)

func TestMainnet_Deneb_Sanity_Slots(t *testing.T) {
	sanity.RunSlotProcessingTests(t, "mainnet")
}
