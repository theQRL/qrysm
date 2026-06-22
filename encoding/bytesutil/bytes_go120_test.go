//go:build go1.20
// +build go1.20

package bytesutil_test

import (
	"testing"
	"unsafe"

	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestUnsafeCastToString(t *testing.T) {
	t.Run("empty slice returns empty string", func(t *testing.T) {
		assert.Equal(t, "", bytesutil.UnsafeCastToString(nil))
		assert.Equal(t, "", bytesutil.UnsafeCastToString([]byte{}))
	})

	t.Run("non-empty slice round-trips", func(t *testing.T) {
		b := []byte("hello world")
		s := bytesutil.UnsafeCastToString(b)
		assert.Equal(t, "hello world", s)
	})

	t.Run("string aliases slice backing memory", func(t *testing.T) {
		b := []byte("alias-me")
		s := bytesutil.UnsafeCastToString(b)
		// The string's data pointer must match the slice's data pointer —
		// proving no copy was made.
		require.Equal(t, unsafe.Pointer(unsafe.SliceData(b)), unsafe.Pointer(unsafe.StringData(s)))
	})
}
