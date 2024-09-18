package iface

import (
	"context"
	"errors"
	"time"

	field_params "github.com/theQRL/qrysm/config/fieldparams"
	validatorserviceconfig "github.com/theQRL/qrysm/config/validator/service"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/dilithium"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/validator/keymanager"
)

// ErrConnectionIssue represents a connection problem.
var ErrConnectionIssue = errors.New("could not connect")

// ValidatorRole defines the validator role.
type ValidatorRole int8

const (
	// RoleUnknown means that the role of the validator cannot be determined.
	RoleUnknown ValidatorRole = iota
	// RoleAttester means that the validator should submit an attestation.
	RoleAttester
	// RoleProposer means that the validator should propose a block.
	RoleProposer
	// RoleAggregator means that the validator should submit an aggregation and proof.
	RoleAggregator
	// RoleSyncCommittee means that the validator should submit a sync committee message.
	RoleSyncCommittee
	// RoleSyncCommitteeAggregator means the validator should aggregate sync committee messages and submit a sync committee contribution.
	RoleSyncCommitteeAggregator
)

// Validator interface defines the primary methods of a validator client.
type Validator interface {
	Done()
	WaitForChainStart(ctx context.Context) error
	WaitForSync(ctx context.Context) error
	WaitForActivation(ctx context.Context, accountsChangedChan chan [][field_params.DilithiumPubkeyLength]byte) error
	CanonicalHeadSlot(ctx context.Context) (primitives.Slot, error)
	NextSlot() <-chan primitives.Slot
	SlotDeadline(slot primitives.Slot) time.Time
	LogValidatorGainsAndLosses(ctx context.Context, slot primitives.Slot) error
	UpdateDuties(ctx context.Context, slot primitives.Slot) error
	RolesAt(ctx context.Context, slot primitives.Slot) (map[[field_params.DilithiumPubkeyLength]byte][]ValidatorRole, error) // validator pubKey -> roles
	SubmitAttestation(ctx context.Context, slot primitives.Slot, pubKey [field_params.DilithiumPubkeyLength]byte)
	ProposeBlock(ctx context.Context, slot primitives.Slot, pubKey [field_params.DilithiumPubkeyLength]byte)
	SubmitAggregateAndProof(ctx context.Context, slot primitives.Slot, pubKey [field_params.DilithiumPubkeyLength]byte)
	SubmitSyncCommitteeMessage(ctx context.Context, slot primitives.Slot, pubKey [field_params.DilithiumPubkeyLength]byte)
	SubmitSignedContributionAndProof(ctx context.Context, slot primitives.Slot, pubKey [field_params.DilithiumPubkeyLength]byte)
	LogAttestationsSubmitted()
	LogSyncCommitteeMessagesSubmitted()
	UpdateDomainDataCaches(ctx context.Context, slot primitives.Slot)
	WaitForKeymanagerInitialization(ctx context.Context) error
	Keymanager() (keymanager.IKeymanager, error)
	ReceiveBlocks(ctx context.Context, connectionErrorChannel chan<- error)
	HandleKeyReload(ctx context.Context, currentKeys [][field_params.DilithiumPubkeyLength]byte) (bool, error)
	CheckDoppelGanger(ctx context.Context) error
	PushProposerSettings(ctx context.Context, km keymanager.IKeymanager, slot primitives.Slot, deadline time.Time) error
	SignValidatorRegistrationRequest(ctx context.Context, signer SigningFunc, newValidatorRegistration *zondpb.ValidatorRegistrationV1) (*zondpb.SignedValidatorRegistrationV1, error)
	ProposerSettings() *validatorserviceconfig.ProposerSettings
	SetProposerSettings(context.Context, *validatorserviceconfig.ProposerSettings) error
}

// SigningFunc interface defines a type for the a function that signs a message
type SigningFunc func(context.Context, *validatorpb.SignRequest) (dilithium.Signature, error)
