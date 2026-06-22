package p2p

import (
	"context"
	"testing"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	dbutil "github.com/theQRL/qrysm/beacon-chain/db/testing"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestCorrect_ActiveValidatorsCount(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig().Copy()
	cfg.ConfigName = "test"

	params.OverrideBeaconConfig(cfg)

	db := dbutil.SetupDB(t)
	s := &Service{
		ctx: context.Background(),
		cfg: &Config{DB: db},
	}
	bState, err := util.NewBeaconStateZond(func(state *qrysmpb.BeaconStateZond) error {
		validators := make([]*qrysmpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
		for i := range validators {
			validators[i] = &qrysmpb.Validator{
				PublicKey:             make([]byte, field_params.MLDSA87PubkeyLength),
				WithdrawalCredentials: make([]byte, 64),
				ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
				Slashed:               false,
			}
		}
		state.Validators = validators
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisData(s.ctx, bState))

	vals, err := s.retrieveActiveValidators()
	assert.NoError(t, err, "genesis state not retrieved")
	assert.Equal(t, int(params.BeaconConfig().MinGenesisActiveValidatorCount), int(vals), "mainnet genesis active count isn't accurate")
	for range 100 {
		require.NoError(t, bState.AppendValidator(&qrysmpb.Validator{
			PublicKey:             make([]byte, field_params.MLDSA87PubkeyLength),
			WithdrawalCredentials: make([]byte, 64),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			Slashed:               false,
		}))
	}
	require.NoError(t, bState.SetSlot(10000))
	require.NoError(t, db.SaveState(s.ctx, bState, [32]byte{'a'}))
	// Reset count
	s.activeValidatorCount = 0

	// Retrieve last archived state.
	vals, err = s.retrieveActiveValidators()
	assert.NoError(t, err, "genesis state not retrieved")
	assert.Equal(t, int(params.BeaconConfig().MinGenesisActiveValidatorCount)+100, int(vals), "mainnet genesis active count isn't accurate")
}

func TestLoggingParameters(_ *testing.T) {
	logGossipParameters("testing", nil)
	logGossipParameters("testing", &pubsub.TopicScoreParams{})
	// Test out actual gossip parameters.
	logGossipParameters("testing", defaultBlockTopicParams())
	p := defaultAggregateSubnetTopicParams(10000)
	logGossipParameters("testing", p)
	p = defaultAggregateTopicParams(10000)
	logGossipParameters("testing", p)
	logGossipParameters("testing", defaultAttesterSlashingTopicParams())
	logGossipParameters("testing", defaultProposerSlashingTopicParams())
	logGossipParameters("testing", defaultVoluntaryExitTopicParams())
}
