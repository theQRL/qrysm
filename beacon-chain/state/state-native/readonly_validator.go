package state_native

import (
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/state"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

var (
	// ErrNilWrappedValidator returns when caller attempts to wrap a nil pointer validator.
	ErrNilWrappedValidator = errors.New("nil validator cannot be wrapped as readonly")
)

// readOnlyValidator returns a wrapper that only allows fields from a validator
// to be read, and prevents any modification of internal validator fields.
type readOnlyValidator struct {
	validator *qrysmpb.Validator
}

var _ = state.ReadOnlyValidator(readOnlyValidator{})

// NewValidator initializes the read only wrapper for validator.
func NewValidator(v *qrysmpb.Validator) (state.ReadOnlyValidator, error) {
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
func (v readOnlyValidator) PublicKey() [field_params.MLDSA87PubkeyLength]byte {
	var pubkey [field_params.MLDSA87PubkeyLength]byte
	copy(pubkey[:], v.validator.PublicKey)
	return pubkey
}

// WithdrawalRecipient returns the withdrawal recipient of the
// read only validator.
func (v readOnlyValidator) WithdrawalRecipient() []byte {
	withdrawalRecipient := make([]byte, len(v.validator.WithdrawalRecipient))
	copy(withdrawalRecipient, v.validator.WithdrawalRecipient)
	return withdrawalRecipient
}

// Slashed returns the read only validator is slashed.
// RandaoCommitment returns the validator's current RANDAO hash-onion
// commitment: the SHA-256 of the next randao_reveal it may put in a block.
func (v readOnlyValidator) RandaoCommitment() [field_params.RandaoCommitmentLength]byte {
	return bytesutil.ToBytes32(v.validator.RandaoCommitment)
}

func (v readOnlyValidator) Slashed() bool {
	return v.validator.Slashed
}

// IsNil returns true if the validator is nil.
func (v readOnlyValidator) IsNil() bool {
	return v.validator == nil
}
