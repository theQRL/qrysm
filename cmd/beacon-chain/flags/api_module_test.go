package flags

import (
	"testing"

	"github.com/theQRL/qrysm/testing/assert"
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

func TestEnableHTTPQRLAPI(t *testing.T) {
	assert.Equal(t, true, EnableHTTPQRLAPI("qrl"))
	assert.Equal(t, true, EnableHTTPQRLAPI("qrl,foo"))
	assert.Equal(t, true, EnableHTTPQRLAPI("foo,qrl"))
	assert.Equal(t, true, EnableHTTPQRLAPI("qrl,qrl"))
	assert.Equal(t, true, EnableHTTPQRLAPI("qRL"))
	assert.Equal(t, false, EnableHTTPQRLAPI("foo"))
	assert.Equal(t, false, EnableHTTPQRLAPI(""))
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
