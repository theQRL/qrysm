package simulator

import (
	"testing"

	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	dbtest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	mockstategen "github.com/theQRL/qrysm/beacon-chain/state/stategen/mock"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func setupService(t *testing.T, params *Parameters) *Simulator {
	slasherDB := dbtest.SetupSlasherDB(t)
	beaconState, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	// We setup validators in the beacon state along with their
	// private keys used to generate valid signatures in generated objects.
	validators := make([]*qrysmpb.Validator, params.NumValidators)
	privKeys := make(map[primitives.ValidatorIndex]ml_dsa_87.MLDSA87Key)
	for valIdx := range validators {
		privKey, err := ml_dsa_87.RandKey()
		require.NoError(t, err)
		privKeys[primitives.ValidatorIndex(valIdx)] = privKey
		validators[valIdx] = &qrysmpb.Validator{
			PublicKey:             privKey.PublicKey().Marshal(),
			WithdrawalCredentials: make([]byte, 64),
		}
	}
	err = beaconState.SetValidators(validators)
	require.NoError(t, err)

	gen := mockstategen.NewMockService()
	gen.AddStateForRoot(beaconState, [32]byte{})
	return &Simulator{
		srvConfig: &ServiceConfig{
			Params:                      params,
			Database:                    slasherDB,
			AttestationStateFetcher:     &mock.ChainService{State: beaconState},
			PrivateKeysByValidatorIndex: privKeys,
			StateGen:                    gen,
		},
	}
}
