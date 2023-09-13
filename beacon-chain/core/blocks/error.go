package blocks

import "github.com/pkg/errors"

var errNilSignedWithdrawalMessage = errors.New("nil SignedDilithiumToExecutionChange message")
var errNilWithdrawalMessage = errors.New("nil DilithiumToExecutionChange message")
var errInvalidDilithiumPrefix = errors.New("withdrawal credential prefix is not a Dilithium prefix")
var errInvalidWithdrawalCredentials = errors.New("withdrawal credentials do not match")
