package testing

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/api/client/builder"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/db"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	v1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
)

// Config defines a config struct for dependencies into the service.
type Config struct {
	BeaconDB db.HeadAccessDatabase
}

// MockBuilderService to mock builder.
type MockBuilderService struct {
	HasConfigured         bool
	PayloadZond           *v1.ExecutionPayloadZond
	ErrSubmitBlindedBlock error
	BidZond               *qrysmpb.SignedBuilderBidZond
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
	case version.Zond:
		w, err := blocks.WrappedExecutionPayloadZond(s.PayloadZond, 0)
		if err != nil {
			return nil, errors.Wrap(err, "could not wrap zond payload")
		}
		return w, s.ErrSubmitBlindedBlock
	default:
		return nil, errors.New("unknown block version for mocking")
	}
}

// GetHeader for mocking.
func (s *MockBuilderService) GetHeader(_ context.Context, slot primitives.Slot, _ [32]byte, _ [field_params.MLDSA87PubkeyLength]byte) (builder.SignedBid, error) {
	return builder.WrappedSignedBuilderBidZond(s.BidZond)
}

// RegistrationByValidatorID returns either the values from the cache or db.
func (s *MockBuilderService) RegistrationByValidatorID(ctx context.Context, id primitives.ValidatorIndex) (*qrysmpb.ValidatorRegistrationV1, error) {
	if s.RegistrationCache != nil {
		return s.RegistrationCache.RegistrationByIndex(id)
	}
	if s.Cfg != nil && s.Cfg.BeaconDB != nil {
		return s.Cfg.BeaconDB.RegistrationByValidatorID(ctx, id)
	}
	return nil, cache.ErrNotFoundRegistration
}

// RegisterValidator for mocking.
func (s *MockBuilderService) RegisterValidator(context.Context, []*qrysmpb.SignedValidatorRegistrationV1) error {
	return s.ErrRegisterValidator
}
