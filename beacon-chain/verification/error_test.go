package verification

import (
	"errors"
	"testing"

	"github.com/theQRL/qrysm/testing/require"
)

func TestAsVerificationFailure(t *testing.T) {
	base := errors.New("signature failed")
	joined := AsVerificationFailure(base)

	// errors.Is must report both the original error and ErrInvalid.
	require.Equal(t, true, errors.Is(joined, ErrInvalid))
	require.Equal(t, true, errors.Is(joined, base))

	// A plain error not joined with ErrInvalid must not satisfy the sentinel.
	require.Equal(t, false, errors.Is(base, ErrInvalid))
}
