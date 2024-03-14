package testing

import (
	"context"

	"github.com/theQRL/qrysm/v4/api/client/builder"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

// MockClient is a mock implementation of BuilderClient.
type MockClient struct {
	RegisteredVals map[[2592]byte]bool
}

// NewClient creates a new, correctly initialized mock.
func NewClient() MockClient {
	return MockClient{RegisteredVals: map[[2592]byte]bool{}}
}

// NodeURL --
func (MockClient) NodeURL() string {
	return ""
}

// GetHeader --
func (MockClient) GetHeader(_ context.Context, _ primitives.Slot, _ [32]byte, _ [field_params.DilithiumPubkeyLength]byte) (builder.SignedBid, error) {
	return nil, nil
}

// RegisterValidator --
func (m MockClient) RegisterValidator(_ context.Context, svr []*zondpb.SignedValidatorRegistrationV1) error {
	for _, r := range svr {
		b := bytesutil.ToBytes2592(r.Message.Pubkey)
		m.RegisteredVals[b] = true
	}
	return nil
}

// SubmitBlindedBlock --
func (MockClient) SubmitBlindedBlock(_ context.Context, _ interfaces.ReadOnlySignedBeaconBlock) (interfaces.ExecutionData, error) {
	return nil, nil
}

// Status --
func (MockClient) Status(_ context.Context) error {
	return nil
}
