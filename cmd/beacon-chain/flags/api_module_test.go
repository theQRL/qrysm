package flags

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/assert"
)

func TestEnableHTTPQrysmAPI(t *testing.T) {
	assert.Equal(t, true, EnableHTTPQrysmAPI("qrysm"))
	assert.Equal(t, true, EnableHTTPQrysmAPI("Qrysm,foo"))
	assert.Equal(t, true, EnableHTTPQrysmAPI("foo,qrysm"))
	assert.Equal(t, true, EnableHTTPQrysmAPI("qrysm,qrysm"))
	assert.Equal(t, true, EnableHTTPQrysmAPI("QrYsM"))
	assert.Equal(t, false, EnableHTTPQrysmAPI("foo"))
	assert.Equal(t, false, EnableHTTPQrysmAPI(""))
}

func TestEnableHTTPZondAPI(t *testing.T) {
	assert.Equal(t, true, EnableHTTPZondAPI("zond"))
	assert.Equal(t, true, EnableHTTPZondAPI("zond,foo"))
	assert.Equal(t, true, EnableHTTPZondAPI("foo,zond"))
	assert.Equal(t, true, EnableHTTPZondAPI("zond,zond"))
	assert.Equal(t, true, EnableHTTPZondAPI("ZonD"))
	assert.Equal(t, false, EnableHTTPZondAPI("foo"))
	assert.Equal(t, false, EnableHTTPZondAPI(""))
}

func TestEnableApi(t *testing.T) {
	assert.Equal(t, true, enableAPI("foo", "foo"))
	assert.Equal(t, true, enableAPI("foo,bar", "foo"))
	assert.Equal(t, true, enableAPI("bar,foo", "foo"))
	assert.Equal(t, true, enableAPI("foo,foo", "foo"))
	assert.Equal(t, true, enableAPI("FoO", "foo"))
	assert.Equal(t, false, enableAPI("bar", "foo"))
	assert.Equal(t, false, enableAPI("", "foo"))
}
