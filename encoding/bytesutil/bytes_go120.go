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
	return [4]byte(PadTo(x, 4))
}

// ToBytes20 is a convenience method for converting a byte slice to a fix
// sized 20 byte array. This method will truncate the input if it is larger
// than 20 bytes.
func ToBytes20(x []byte) [20]byte {
	return [20]byte(PadTo(x, 20))
}

// ToBytes32 is a convenience method for converting a byte slice to a fix
// sized 32 byte array. This method will truncate the input if it is larger
// than 32 bytes.
func ToBytes32(x []byte) [32]byte {
	return [32]byte(PadTo(x, 32))
}

// ToBytes48 is a convenience method for converting a byte slice to a fix
// sized 48 byte array. This method will truncate the input if it is larger
// than 48 bytes.
func ToBytes48(x []byte) [48]byte {
	return [48]byte(PadTo(x, 48))
}

// ToBytes2592 is a convenience method for converting a byte slice to a fix
// sized 2592 byte array. This method will truncate the input if it is larger
// than 2592 bytes.
func ToBytes2592(x []byte) [2592]byte {
	return [2592]byte(PadTo(x, 2592))
}

// ToBytes4627 is a convenience method for converting a byte slice to a fix
// sized 4627 byte array. This method will truncate the input if it is larger
// than 4627 bytes.
func ToBytes4627(x []byte) [4627]byte {
	return [4627]byte(PadTo(x, 4627))
}

// ToBytes64 is a convenience method for converting a byte slice to a fix
// sized 64 byte array. This method will truncate the input if it is larger
// than 64 bytes.
func ToBytes64(x []byte) [64]byte {
	return [64]byte(PadTo(x, 64))
}

// ToBytes96 is a convenience method for converting a byte slice to a fix
// sized 96 byte array. This method will truncate the input if it is larger
// than 96 bytes.
func ToBytes96(x []byte) [96]byte {
	return [96]byte(PadTo(x, 96))
}
