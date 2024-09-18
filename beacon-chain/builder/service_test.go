package builder

import (
	"context"
	"testing"
	"time"

	buildertesting "github.com/theQRL/qrysm/api/client/builder/testing"
	blockchainTesting "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	dbtesting "github.com/theQRL/qrysm/beacon-chain/db/testing"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zond "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func Test_NewServiceWithBuilder(t *testing.T) {
	s, err := NewService(context.Background(), WithBuilderClient(&buildertesting.MockClient{}))
	require.NoError(t, err)
	assert.Equal(t, true, s.Configured())
}

func Test_NewServiceWithoutBuilder(t *testing.T) {
	s, err := NewService(context.Background())
	require.NoError(t, err)
	assert.Equal(t, false, s.Configured())
}

func Test_RegisterValidator(t *testing.T) {
	ctx := context.Background()
	db := dbtesting.SetupDB(t)
	headFetcher := &blockchainTesting.ChainService{}
	builder := buildertesting.NewClient()
	s, err := NewService(ctx, WithDatabase(db), WithHeadFetcher(headFetcher), WithBuilderClient(&builder))
	require.NoError(t, err)
	pubkey := bytesutil.ToBytes2592([]byte("pubkey"))
	var feeRecipient [20]byte
	require.NoError(t, s.RegisterValidator(ctx, []*zond.SignedValidatorRegistrationV1{{Message: &zond.ValidatorRegistrationV1{Pubkey: pubkey[:], FeeRecipient: feeRecipient[:]}}}))
	assert.Equal(t, true, builder.RegisteredVals[pubkey])
}

func Test_RegisterValidator_WithCache(t *testing.T) {
	ctx := context.Background()
	headFetcher := &blockchainTesting.ChainService{}
	builder := buildertesting.NewClient()
	s, err := NewService(ctx, WithRegistrationCache(), WithHeadFetcher(headFetcher), WithBuilderClient(&builder))
	require.NoError(t, err)
	pubkey := bytesutil.ToBytes2592([]byte("pubkey"))
	var feeRecipient [20]byte
	reg := &zond.ValidatorRegistrationV1{Pubkey: pubkey[:], Timestamp: uint64(time.Now().UTC().Unix()), FeeRecipient: feeRecipient[:]}
	require.NoError(t, s.RegisterValidator(ctx, []*zond.SignedValidatorRegistrationV1{{Message: reg}}))
	registration, err := s.registrationCache.RegistrationByIndex(0)
	require.NoError(t, err)
	require.DeepEqual(t, reg, registration)
}

func Test_BuilderMethodsWithouClient(t *testing.T) {
	s, err := NewService(context.Background())
	require.NoError(t, err)
	assert.Equal(t, false, s.Configured())

	_, err = s.GetHeader(context.Background(), 0, [32]byte{}, [field_params.DilithiumPubkeyLength]byte{})
	assert.ErrorContains(t, ErrNoBuilder.Error(), err)

	_, err = s.SubmitBlindedBlock(context.Background(), nil)
	assert.ErrorContains(t, ErrNoBuilder.Error(), err)

	err = s.RegisterValidator(context.Background(), nil)
	assert.ErrorContains(t, ErrNoBuilder.Error(), err)
}
