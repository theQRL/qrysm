package bytesutil

import (
	"fmt"
	"regexp"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common/hexutil"
)

var hexRegex = regexp.MustCompile("^0x[0-9a-fA-F]+$")

// IsHex checks whether the byte array is a hex number prefixed with '0x'.
func IsHex(b []byte) bool {
	if b == nil {
		return false
	}
	return hexRegex.Match(b)
}

// DecodeHexWithLength takes a string and a length in bytes,
// and validates whether the string is a hex and has the correct length.
func DecodeHexWithLength(s string, length int) ([]byte, error) {
	if len(s) > 2*length+2 {
		return nil, fmt.Errorf("%s is greather than length %d bytes", s, length)
	}
	bytes, err := hexutil.Decode(s)
	if err != nil {
		return nil, errors.Wrapf(err, "%s is not a valid hex", s)
	}
	if len(bytes) != length {
		return nil, fmt.Errorf("length of %s is not %d bytes", s, length)
	}
	return bytes, nil
}
