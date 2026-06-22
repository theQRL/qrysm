package fdlimits_test

import (
	"testing"

	gqrlLimit "github.com/theQRL/go-qrl/common/fdlimit"
	"github.com/theQRL/qrysm/runtime/fdlimits"
	"github.com/theQRL/qrysm/testing/assert"
)

func TestSetMaxFdLimits(t *testing.T) {
	assert.NoError(t, fdlimits.SetMaxFdLimits())

	curr, err := gqrlLimit.Current()
	assert.NoError(t, err)

	max, err := gqrlLimit.Maximum()
	assert.NoError(t, err)

	assert.Equal(t, max, curr, "current and maximum file descriptor limits do not match up.")

}
