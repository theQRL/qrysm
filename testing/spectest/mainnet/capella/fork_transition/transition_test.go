package fork_transition

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/capella/fork"
)

func TestMainnet_Capella_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "mainnet")
}
