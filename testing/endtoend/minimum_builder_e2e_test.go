package endtoend

import (
	"testing"

	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/endtoend/types"
)

func TestEndToEnd_MinimalConfig_WithBuilder(t *testing.T) {
	e2eMinimal(t, version.Capella /*, types.WithCheckpointSync()*/, types.WithBuilder()).run()
}
