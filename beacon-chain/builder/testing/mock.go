package testing

import (
	"context"

	"github.com/pkg/errors"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/api/client/builder"
	"github.com/theQRL/qrysm/v4/beacon-chain/cache"
	"github.com/theQRL/qrysm/v4/beacon-chain/db"
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
	Payload               *v1.ExecutionPayload
	PayloadCapella        *v1.ExecutionPayloadCapella
	ErrSubmitBlindedBlock error
	Bid                   *zondpb.SignedBuilderBid
	BidCapella            *zondpb.SignedBuilderBidCapella
	RegistrationCache     *cache.RegistrationCache
	ErrGetHeader          error
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
	case version.Bellatrix:
		w, err := blocks.WrappedExecutionPayload(s.Payload)
		if err != nil {
			return nil, errors.Wrap(err, "could not wrap payload")
		}
		return w, s.ErrSubmitBlindedBlock
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
func (s *MockBuilderService) GetHeader(_ context.Context, slot primitives.Slot, _ [32]byte, _ [dilithium2.CryptoPublicKeyBytes]byte) (builder.SignedBid, error) {
	// TODO(rgeraldes24): come back to this one once we cover the beacon chain unit tests
	/*
		w, err := builder.WrappedSignedBuilderBid(s.Bid)
		if err != nil {
			return nil, errors.Wrap(err, "could not wrap capella bid")
		}
		return w, s.ErrGetHeader
	*/

	return nil, nil
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
