package fdlimits_test

import (
	"testing"

	gzondLimit "github.com/theQRL/go-zond/common/fdlimit"
	"github.com/theQRL/qrysm/v4/runtime/fdlimits"
	"github.com/theQRL/qrysm/v4/testing/assert"
)

func TestSetMaxFdLimits(t *testing.T) {
	assert.NoError(t, fdlimits.SetMaxFdLimits())

	curr, err := gzondLimit.Current()
	assert.NoError(t, err)

	max, err := gzondLimit.Maximum()
	assert.NoError(t, err)

	assert.Equal(t, max, curr, "current and maximum file descriptor limits do not match up.")

}
