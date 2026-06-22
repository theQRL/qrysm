package endtoend

import (
	"testing"

	"github.com/theQRL/qrysm/runtime/version"
)

func TestEndToEnd_MinimalConfig(t *testing.T) {
	e2eMinimal(t, version.Zond /*,types.WithCheckpointSync()*/).run()
}
