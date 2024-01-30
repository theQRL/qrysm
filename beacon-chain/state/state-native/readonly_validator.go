package state_native

import (
	"github.com/pkg/errors"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

var (
	// ErrNilWrappedValidator returns when caller attempts to wrap a nil pointer validator.
	ErrNilWrappedValidator = errors.New("nil validator cannot be wrapped as readonly")
)

// readOnlyValidator returns a wrapper that only allows fields from a validator
// to be read, and prevents any modification of internal validator fields.
type readOnlyValidator struct {
	validator *zondpb.Validator
}

var _ = state.ReadOnlyValidator(readOnlyValidator{})

// NewValidator initializes the read only wrapper for validator.
func NewValidator(v *zondpb.Validator) (state.ReadOnlyValidator, error) {
	if v == nil {
		return nil, ErrNilWrappedValidator
	}
	rov := readOnlyValidator{
		validator: v,
	}
	return rov, nil
}

// EffectiveBalance returns the effective balance of the
// read only validator.
func (v readOnlyValidator) EffectiveBalance() uint64 {
	return v.validator.EffectiveBalance
}

// ActivationEligibilityEpoch returns the activation eligibility epoch of the
// read only validator.
func (v readOnlyValidator) ActivationEligibilityEpoch() primitives.Epoch {
	return v.validator.ActivationEligibilityEpoch
}

// ActivationEpoch returns the activation epoch of the
// read only validator.
func (v readOnlyValidator) ActivationEpoch() primitives.Epoch {
	return v.validator.ActivationEpoch
}

// WithdrawableEpoch returns the withdrawable epoch of the
// read only validator.
func (v readOnlyValidator) WithdrawableEpoch() primitives.Epoch {
	return v.validator.WithdrawableEpoch
}

// ExitEpoch returns the exit epoch of the
// read only validator.
func (v readOnlyValidator) ExitEpoch() primitives.Epoch {
	return v.validator.ExitEpoch
}

// PublicKey returns the public key of the
// read only validator.
func (v readOnlyValidator) PublicKey() [dilithium2.CryptoPublicKeyBytes]byte {
	var pubkey [dilithium2.CryptoPublicKeyBytes]byte
	copy(pubkey[:], v.validator.PublicKey)
	return pubkey
}

// WithdrawalCredentials returns the withdrawal credentials of the
// read only validator.
func (v readOnlyValidator) WithdrawalCredentials() []byte {
	creds := make([]byte, len(v.validator.WithdrawalCredentials))
	copy(creds, v.validator.WithdrawalCredentials)
	return creds
}

// Slashed returns the read only validator is slashed.
func (v readOnlyValidator) Slashed() bool {
	return v.validator.Slashed
}

// IsNil returns true if the validator is nil.
func (v readOnlyValidator) IsNil() bool {
	return v.validator == nil
}
