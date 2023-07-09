package endtoend

import (
	"testing"

	"github.com/cyyber/qrysm/v4/runtime/version"
	"github.com/cyyber/qrysm/v4/testing/endtoend/types"
)

func TestEndToEnd_MinimalConfig_WithBuilder(t *testing.T) {
	r := e2eMinimal(t, version.Phase0, types.WithCheckpointSync(), types.WithBuilder())
	r.run()
}
