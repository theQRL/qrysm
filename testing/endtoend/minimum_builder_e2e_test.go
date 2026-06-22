package endtoend

import (
	"testing"

	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/endtoend/types"
)

func TestEndToEnd_MinimalConfig_WithBuilder(t *testing.T) {
	e2eMinimal(t, version.Zond /*, types.WithCheckpointSync()*/, types.WithBuilder()).run()
}
