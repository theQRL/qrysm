package fork_transition

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/deneb/fork"
)

func TestMinimal_Deneb_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "minimal")
}
