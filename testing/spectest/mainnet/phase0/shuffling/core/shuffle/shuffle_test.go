package shuffle

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/phase0/shuffling/core/shuffle"
)

func TestMainnet_Phase0_Shuffling_Core_Shuffle(t *testing.T) {
	shuffle.RunShuffleTests(t, "mainnet")
}
