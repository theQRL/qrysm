package iface

import (
	"context"
	"errors"
	"time"

	validatorserviceconfig "github.com/cyyber/qrysm/v4/config/validator/service"
	"github.com/cyyber/qrysm/v4/consensus-types/primitives"
	"github.com/cyyber/qrysm/v4/crypto/dilithium"
	ethpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
	validatorpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1/validator-client"
	"github.com/cyyber/qrysm/v4/validator/keymanager"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
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
	// RoleSyncCommitteeAggregator means the valiator should aggregate sync committee messages and submit a sync committee contribution.
	RoleSyncCommitteeAggregator
)

// Validator interface defines the primary methods of a validator client.
type Validator interface {
	Done()
	WaitForChainStart(ctx context.Context) error
	WaitForSync(ctx context.Context) error
	WaitForActivation(ctx context.Context, accountsChangedChan chan [][dilithium2.CryptoPublicKeyBytes]byte) error
	CanonicalHeadSlot(ctx context.Context) (primitives.Slot, error)
	NextSlot() <-chan primitives.Slot
	SlotDeadline(slot primitives.Slot) time.Time
	LogValidatorGainsAndLosses(ctx context.Context, slot primitives.Slot) error
	UpdateDuties(ctx context.Context, slot primitives.Slot) error
	RolesAt(ctx context.Context, slot primitives.Slot) (map[[dilithium2.CryptoPublicKeyBytes]byte][]ValidatorRole, error) // validator pubKey -> roles
	SubmitAttestation(ctx context.Context, slot primitives.Slot, pubKey [dilithium2.CryptoPublicKeyBytes]byte)
	ProposeBlock(ctx context.Context, slot primitives.Slot, pubKey [dilithium2.CryptoPublicKeyBytes]byte)
	SubmitAggregateAndProof(ctx context.Context, slot primitives.Slot, pubKey [dilithium2.CryptoPublicKeyBytes]byte)
	SubmitSyncCommitteeMessage(ctx context.Context, slot primitives.Slot, pubKey [dilithium2.CryptoPublicKeyBytes]byte)
	SubmitSignedContributionAndProof(ctx context.Context, slot primitives.Slot, pubKey [dilithium2.CryptoPublicKeyBytes]byte)
	LogAttestationsSubmitted()
	LogSyncCommitteeMessagesSubmitted()
	UpdateDomainDataCaches(ctx context.Context, slot primitives.Slot)
	WaitForKeymanagerInitialization(ctx context.Context) error
	AllValidatorsAreExited(ctx context.Context) (bool, error)
	Keymanager() (keymanager.IKeymanager, error)
	ReceiveBlocks(ctx context.Context, connectionErrorChannel chan<- error)
	HandleKeyReload(ctx context.Context, currentKeys [][dilithium2.CryptoPublicKeyBytes]byte) (bool, error)
	CheckDoppelGanger(ctx context.Context) error
	PushProposerSettings(ctx context.Context, km keymanager.IKeymanager, deadline time.Time) error
	SignValidatorRegistrationRequest(ctx context.Context, signer SigningFunc, newValidatorRegistration *ethpb.ValidatorRegistrationV1) (*ethpb.SignedValidatorRegistrationV1, error)
	ProposerSettings() *validatorserviceconfig.ProposerSettings
	SetProposerSettings(*validatorserviceconfig.ProposerSettings)
}

// SigningFunc interface defines a type for the a function that signs a message
type SigningFunc func(context.Context, *validatorpb.SignRequest) (dilithium.Signature, error)
