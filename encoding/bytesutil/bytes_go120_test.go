//go:build go1.20
// +build go1.20

package bytesutil_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"unsafe"

	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestToBytes_PreservesBackingArray(t *testing.T) {
	for size, convert := range map[int]any{
		4:  bytesutil.ToBytes4,
		20: bytesutil.ToBytes20, 32: bytesutil.ToBytes32, 48: bytesutil.ToBytes48,
		64: bytesutil.ToBytes64, 96: bytesutil.ToBytes96,
		2592: bytesutil.ToBytes2592, 4627: bytesutil.ToBytes4627,
	} {
		for _, length := range []int{0, 1, size - 1, size, size + 1} {
			t.Run(fmt.Sprintf("%d/%d", size, length), func(t *testing.T) {
				backing := bytes.Repeat([]byte{0x7b}, size+1)
				want := bytes.Clone(backing)
				result := reflect.ValueOf(convert).Call([]reflect.Value{reflect.ValueOf(backing[:length])})[0]
				if !bytes.Equal(backing, want) {
					t.Fatal("conversion changed the input backing array")
				}
				for i := 0; i < size; i++ {
					var expected uint64
					if i < length {
						expected = 0x7b
					}
					if result.Index(i).Uint() != expected {
						t.Fatalf("incorrect result at byte %d", i)
					}
				}
			})
		}
	}
}

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
