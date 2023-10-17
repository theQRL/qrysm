package fork_transition

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/altair/fork"
)

func TestMainnet_Altair_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "mainnet")
}
