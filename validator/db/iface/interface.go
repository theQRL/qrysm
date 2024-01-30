// Package iface defines an interface for the validator database.
package iface

import (
	"context"
	"io"

	"github.com/theQRL/go-qrllib/dilithium"
	validatorServiceConfig "github.com/theQRL/qrysm/v4/config/validator/service"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/monitoring/backup"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/validator/db/kv"
)

// Ensure the kv store implements the interface.
var _ = ValidatorDB(&kv.Store{})

// ValidatorDB defines the necessary methods for a Qrysm validator DB.
type ValidatorDB interface {
	io.Closer
	backup.BackupExporter
	DatabasePath() string
	ClearDB() error
	RunUpMigrations(ctx context.Context) error
	RunDownMigrations(ctx context.Context) error
	UpdatePublicKeysBuckets(publicKeys [][dilithium.CryptoPublicKeyBytes]byte) error

	// Genesis information related methods.
	GenesisValidatorsRoot(ctx context.Context) ([]byte, error)
	SaveGenesisValidatorsRoot(ctx context.Context, genValRoot []byte) error

	// Proposer protection related methods.
	HighestSignedProposal(ctx context.Context, publicKey [dilithium.CryptoPublicKeyBytes]byte) (primitives.Slot, bool, error)
	LowestSignedProposal(ctx context.Context, publicKey [dilithium.CryptoPublicKeyBytes]byte) (primitives.Slot, bool, error)
	ProposalHistoryForPubKey(ctx context.Context, publicKey [dilithium.CryptoPublicKeyBytes]byte) ([]*kv.Proposal, error)
	ProposalHistoryForSlot(ctx context.Context, publicKey [dilithium.CryptoPublicKeyBytes]byte, slot primitives.Slot) ([32]byte, bool, error)
	SaveProposalHistoryForSlot(ctx context.Context, pubKey [dilithium.CryptoPublicKeyBytes]byte, slot primitives.Slot, signingRoot []byte) error
	ProposedPublicKeys(ctx context.Context) ([][dilithium.CryptoPublicKeyBytes]byte, error)

	// Attester protection related methods.
	// Methods to store and read blacklisted public keys from EIP-3076
	// slashing protection imports.
	EIPImportBlacklistedPublicKeys(ctx context.Context) ([][dilithium.CryptoPublicKeyBytes]byte, error)
	SaveEIPImportBlacklistedPublicKeys(ctx context.Context, publicKeys [][dilithium.CryptoPublicKeyBytes]byte) error
	SigningRootAtTargetEpoch(ctx context.Context, publicKey [dilithium.CryptoPublicKeyBytes]byte, target primitives.Epoch) ([32]byte, error)
	LowestSignedTargetEpoch(ctx context.Context, publicKey [dilithium.CryptoPublicKeyBytes]byte) (primitives.Epoch, bool, error)
	LowestSignedSourceEpoch(ctx context.Context, publicKey [dilithium.CryptoPublicKeyBytes]byte) (primitives.Epoch, bool, error)
	AttestedPublicKeys(ctx context.Context) ([][dilithium.CryptoPublicKeyBytes]byte, error)
	CheckSlashableAttestation(
		ctx context.Context, pubKey [dilithium.CryptoPublicKeyBytes]byte, signingRoot [32]byte, att *zondpb.IndexedAttestation,
	) (kv.SlashingKind, error)
	SaveAttestationForPubKey(
		ctx context.Context, pubKey [dilithium.CryptoPublicKeyBytes]byte, signingRoot [32]byte, att *zondpb.IndexedAttestation,
	) error
	SaveAttestationsForPubKey(
		ctx context.Context, pubKey [dilithium.CryptoPublicKeyBytes]byte, signingRoots [][32]byte, atts []*zondpb.IndexedAttestation,
	) error
	AttestationHistoryForPubKey(
		ctx context.Context, pubKey [dilithium.CryptoPublicKeyBytes]byte,
	) ([]*kv.AttestationRecord, error)

	// Graffiti ordered index related methods
	SaveGraffitiOrderedIndex(ctx context.Context, index uint64) error
	GraffitiOrderedIndex(ctx context.Context, fileHash [32]byte) (uint64, error)

	// ProposerSettings related methods
	ProposerSettings(context.Context) (*validatorServiceConfig.ProposerSettings, error)
	ProposerSettingsExists(ctx context.Context) (bool, error)
	UpdateProposerSettingsDefault(context.Context, *validatorServiceConfig.ProposerOption) error
	UpdateProposerSettingsForPubkey(context.Context, [dilithium.CryptoPublicKeyBytes]byte, *validatorServiceConfig.ProposerOption) error
	SaveProposerSettings(ctx context.Context, settings *validatorServiceConfig.ProposerSettings) error
}
