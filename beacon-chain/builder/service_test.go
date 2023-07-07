package builder

import (
	"context"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"testing"

	buildertesting "github.com/cyyber/qrysm/v4/api/client/builder/testing"
	blockchainTesting "github.com/cyyber/qrysm/v4/beacon-chain/blockchain/testing"
	dbtesting "github.com/cyyber/qrysm/v4/beacon-chain/db/testing"
	"github.com/cyyber/qrysm/v4/encoding/bytesutil"
	eth "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/cyyber/qrysm/v4/testing/assert"
	"github.com/cyyber/qrysm/v4/testing/require"
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
	pubkey := bytesutil.ToBytes48([]byte("pubkey"))
	var feeRecipient [20]byte
	require.NoError(t, s.RegisterValidator(ctx, []*eth.SignedValidatorRegistrationV1{{Message: &eth.ValidatorRegistrationV1{Pubkey: pubkey[:], FeeRecipient: feeRecipient[:]}}}))
	assert.Equal(t, true, builder.RegisteredVals[pubkey])
}

func Test_BuilderMethodsWithouClient(t *testing.T) {
	s, err := NewService(context.Background())
	require.NoError(t, err)
	assert.Equal(t, false, s.Configured())

	_, err = s.GetHeader(context.Background(), 0, [32]byte{}, [dilithium2.CryptoPublicKeyBytes]byte{})
	assert.ErrorContains(t, ErrNoBuilder.Error(), err)

	_, err = s.SubmitBlindedBlock(context.Background(), nil)
	assert.ErrorContains(t, ErrNoBuilder.Error(), err)

	err = s.RegisterValidator(context.Background(), nil)
	assert.ErrorContains(t, ErrNoBuilder.Error(), err)
}
