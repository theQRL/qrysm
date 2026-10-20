// Package verification holds shared verification-failure sentinels.
//
// Other packages join their domain-specific errors with ErrInvalid so peer
// scoring code in sync paths can detect verification failures with a single
// errors.Is check without depending on the originating package.
package verification

import "errors"

// ErrInvalid is a general-purpose verification failure that can be wrapped or
// joined to indicate a verification failure that should impact peer scoring.
var ErrInvalid = errors.New("verification failure")

// AsVerificationFailure joins the given error with ErrInvalid so it can be
// tested with errors.Is(err, ErrInvalid).
func AsVerificationFailure(err error) error {
	return errors.Join(ErrInvalid, err)
}
