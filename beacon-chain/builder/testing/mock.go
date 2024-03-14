package testing

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/api/client/builder"
	"github.com/theQRL/qrysm/v4/beacon-chain/cache"
	"github.com/theQRL/qrysm/v4/beacon-chain/db"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	v1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
)

// Config defines a config struct for dependencies into the service.
type Config struct {
	BeaconDB db.HeadAccessDatabase
}

// MockBuilderService to mock builder.
type MockBuilderService struct {
	HasConfigured         bool
	PayloadCapella        *v1.ExecutionPayloadCapella
	ErrSubmitBlindedBlock error
	BidCapella            *zondpb.SignedBuilderBidCapella
	RegistrationCache     *cache.RegistrationCache
	ErrRegisterValidator  error
	Cfg                   *Config
}

// Configured for mocking.
func (s *MockBuilderService) Configured() bool {
	return s.HasConfigured
}

// SubmitBlindedBlock for mocking.
func (s *MockBuilderService) SubmitBlindedBlock(_ context.Context, b interfaces.ReadOnlySignedBeaconBlock) (interfaces.ExecutionData, error) {
	switch b.Version() {
	case version.Capella:
		w, err := blocks.WrappedExecutionPayloadCapella(s.PayloadCapella, 0)
		if err != nil {
			return nil, errors.Wrap(err, "could not wrap capella payload")
		}
		return w, s.ErrSubmitBlindedBlock
	default:
		return nil, errors.New("unknown block version for mocking")
	}
}

// GetHeader for mocking.
func (s *MockBuilderService) GetHeader(_ context.Context, slot primitives.Slot, _ [32]byte, _ [field_params.DilithiumPubkeyLength]byte) (builder.SignedBid, error) {
	return builder.WrappedSignedBuilderBidCapella(s.BidCapella)
}

// RegistrationByValidatorID returns either the values from the cache or db.
func (s *MockBuilderService) RegistrationByValidatorID(ctx context.Context, id primitives.ValidatorIndex) (*zondpb.ValidatorRegistrationV1, error) {
	if s.RegistrationCache != nil {
		return s.RegistrationCache.RegistrationByIndex(id)
	}
	if s.Cfg.BeaconDB != nil {
		return s.Cfg.BeaconDB.RegistrationByValidatorID(ctx, id)
	}
	return nil, cache.ErrNotFoundRegistration
}

// RegisterValidator for mocking.
func (s *MockBuilderService) RegisterValidator(context.Context, []*zondpb.SignedValidatorRegistrationV1) error {
	return s.ErrRegisterValidator
}
