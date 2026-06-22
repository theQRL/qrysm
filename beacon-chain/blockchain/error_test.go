package blockchain

import (
	stderrors "errors"
	"testing"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/verification"
	"github.com/theQRL/qrysm/testing/require"
)

func TestIsInvalidBlock(t *testing.T) {
	require.Equal(t, true, IsInvalidBlock(ErrInvalidPayload)) // Already wrapped.
	err := invalidBlock{error: ErrInvalidPayload}
	require.Equal(t, true, IsInvalidBlock(err))

	newErr := errors.Wrap(err, "wrap me")
	require.Equal(t, true, IsInvalidBlock(newErr))
	require.DeepEqual(t, [][32]byte(nil), InvalidAncestorRoots(err))
}

func TestInvalidBlockRoot(t *testing.T) {
	require.Equal(t, [32]byte{}, InvalidBlockRoot(ErrUndefinedExecutionEngineError))
	require.Equal(t, [32]byte{}, InvalidBlockRoot(ErrInvalidPayload))

	err := invalidBlock{error: ErrInvalidPayload, root: [32]byte{'a'}}
	require.Equal(t, [32]byte{'a'}, InvalidBlockRoot(err))
	require.DeepEqual(t, [][32]byte(nil), InvalidAncestorRoots(err))

	newErr := errors.Wrap(err, "wrap me")
	require.Equal(t, [32]byte{'a'}, InvalidBlockRoot(newErr))
}

func TestInvalidBlock_UnwrapsToVerificationErrInvalid(t *testing.T) {
	// Bare invalidBlock should be detected as a verification failure so peer
	// scoring code can downscore the providing peer.
	err := invalidBlock{error: errors.New("bad block")}
	require.Equal(t, true, stderrors.Is(err, verification.ErrInvalid))

	// Wrapping with pkg/errors must preserve the verification.ErrInvalid join.
	wrapped := errors.Wrap(err, "outer context")
	require.Equal(t, true, stderrors.Is(wrapped, verification.ErrInvalid))

	// Errors not related to invalid blocks must not satisfy ErrInvalid.
	require.Equal(t, false, stderrors.Is(ErrUndefinedExecutionEngineError, verification.ErrInvalid))
}

func TestInvalidRoots(t *testing.T) {
	roots := [][32]byte{{'d'}, {'b'}, {'c'}}
	err := invalidBlock{error: ErrInvalidPayload, root: [32]byte{'a'}, invalidAncestorRoots: roots}

	require.Equal(t, true, IsInvalidBlock(err))
	require.Equal(t, [32]byte{'a'}, InvalidBlockRoot(err))
	require.DeepEqual(t, roots, InvalidAncestorRoots(err))

	newErr := errors.Wrap(err, "wrap me")
	require.Equal(t, true, IsInvalidBlock(err))
	require.Equal(t, [32]byte{'a'}, InvalidBlockRoot(newErr))
	require.DeepEqual(t, roots, InvalidAncestorRoots(newErr))
}
