//go:build go1.20
// +build go1.20

package bytesutil

import "unsafe"

// These methods use go1.20 syntax to convert a byte slice to a fixed size array.

// UnsafeCastToString returns a string aliasing the supplied byte slice's
// backing memory — no allocation and no copy. The caller MUST guarantee the
// byte slice is not mutated for the lifetime of the returned string;
// modifying it afterwards breaks Go's string-immutability invariant. Intended
// for hot paths where the slice is already final (e.g. a hash output) and
// will not be touched again.
func UnsafeCastToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// ToBytes4 is a convenience method for converting a byte slice to a fix
// sized 4 byte array. This method will truncate the input if it is larger
// than 4 bytes.
func ToBytes4(x []byte) [4]byte {
	if len(x) < 4 {
		var out [4]byte
		copy(out[:], x)
		return out
	}
	return [4]byte(x)
}

// ToBytes20 is a convenience method for converting a byte slice to a fix
// sized 20 byte array. This method will truncate the input if it is larger
// than 20 bytes.
func ToBytes20(x []byte) [20]byte {
	if len(x) < 20 {
		var out [20]byte
		copy(out[:], x)
		return out
	}
	return [20]byte(x)
}

// ToBytes32 is a convenience method for converting a byte slice to a fix
// sized 32 byte array. This method will truncate the input if it is larger
// than 32 bytes.
func ToBytes32(x []byte) [32]byte {
	if len(x) < 32 {
		var out [32]byte
		copy(out[:], x)
		return out
	}
	return [32]byte(x)
}

// ToBytes48 is a convenience method for converting a byte slice to a fix
// sized 48 byte array. This method will truncate the input if it is larger
// than 48 bytes.
func ToBytes48(x []byte) [48]byte {
	if len(x) < 48 {
		var out [48]byte
		copy(out[:], x)
		return out
	}
	return [48]byte(x)
}

// ToBytes2592 is a convenience method for converting a byte slice to a fix
// sized 2592 byte array. This method will truncate the input if it is larger
// than 2592 bytes.
func ToBytes2592(x []byte) [2592]byte {
	if len(x) < 2592 {
		var out [2592]byte
		copy(out[:], x)
		return out
	}
	return [2592]byte(x)
}

// ToBytes4627 is a convenience method for converting a byte slice to a fix
// sized 4627 byte array. This method will truncate the input if it is larger
// than 4627 bytes.
func ToBytes4627(x []byte) [4627]byte {
	if len(x) < 4627 {
		var out [4627]byte
		copy(out[:], x)
		return out
	}
	return [4627]byte(x)
}

// ToBytes64 is a convenience method for converting a byte slice to a fix
// sized 64 byte array. This method will truncate the input if it is larger
// than 64 bytes.
func ToBytes64(x []byte) [64]byte {
	if len(x) < 64 {
		var out [64]byte
		copy(out[:], x)
		return out
	}
	return [64]byte(x)
}

// ToBytes96 is a convenience method for converting a byte slice to a fix
// sized 96 byte array. This method will truncate the input if it is larger
// than 96 bytes.
func ToBytes96(x []byte) [96]byte {
	if len(x) < 96 {
		var out [96]byte
		copy(out[:], x)
		return out
	}
	return [96]byte(x)
}
