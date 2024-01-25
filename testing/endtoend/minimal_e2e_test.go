package endtoend

import (
	"testing"

	"github.com/theQRL/qrysm/v4/runtime/version"
)

func TestEndToEnd_MinimalConfig(t *testing.T) {
	e2eMinimal(t, version.Capella /*,types.WithCheckpointSync()*/).run()
}
