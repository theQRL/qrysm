package blocks_test

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestProcessVoluntaryExits_NotActiveLongEnoughToExit(t *testing.T) {
	exits := []*qrysmpb.SignedVoluntaryExit{
		{
			Exit: &qrysmpb.VoluntaryExit{
				ValidatorIndex: 0,
				Epoch:          0,
			},
		},
	}
	registry := []*qrysmpb.Validator{
		{
			ExitEpoch: params.BeaconConfig().FarFutureEpoch,
		},
	}
	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: registry,
		Slot:       10,
	})
	require.NoError(t, err)
	b := util.NewBeaconBlockZond()
	b.Block = &qrysmpb.BeaconBlockZond{
		Body: &qrysmpb.BeaconBlockBodyZond{
			VoluntaryExits: exits,
		},
	}

	want := "validator has not been active long enough to exit"
	_, err = blocks.ProcessVoluntaryExits(context.Background(), state, b.Block.Body.VoluntaryExits)
	assert.ErrorContains(t, want, err)
}

func TestProcessVoluntaryExits_ExitAlreadySubmitted(t *testing.T) {
	exits := []*qrysmpb.SignedVoluntaryExit{
		{
			Exit: &qrysmpb.VoluntaryExit{
				Epoch: 10,
			},
		},
	}
	registry := []*qrysmpb.Validator{
		{
			ExitEpoch: 10,
		},
	}
	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: registry,
		Slot:       0,
	})
	require.NoError(t, err)
	b := util.NewBeaconBlockZond()
	b.Block = &qrysmpb.BeaconBlockZond{
		Body: &qrysmpb.BeaconBlockBodyZond{
			VoluntaryExits: exits,
		},
	}

	want := "validator with index 0 has already submitted an exit, which will take place at epoch: 10"
	_, err = blocks.ProcessVoluntaryExits(context.Background(), state, b.Block.Body.VoluntaryExits)
	assert.ErrorContains(t, want, err)
}

func TestProcessVoluntaryExits_AppliesCorrectStatus(t *testing.T) {
	exits := []*qrysmpb.SignedVoluntaryExit{
		{
			Exit: &qrysmpb.VoluntaryExit{
				ValidatorIndex: 0,
				Epoch:          0,
			},
		},
	}
	registry := []*qrysmpb.Validator{
		{
			ExitEpoch:       params.BeaconConfig().FarFutureEpoch,
			ActivationEpoch: 0,
		},
	}
	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: registry,
		Fork: &qrysmpb.Fork{
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
		},
		Slot: params.BeaconConfig().SlotsPerEpoch * 5,
	})
	require.NoError(t, err)
	err = state.SetSlot(state.Slot() + params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().ShardCommitteePeriod)))
	require.NoError(t, err)

	priv, err := ml_dsa_87.RandKey()
	require.NoError(t, err)

	val, err := state.ValidatorAtIndex(0)
	require.NoError(t, err)
	val.PublicKey = priv.PublicKey().Marshal()
	require.NoError(t, state.UpdateValidatorAtIndex(0, val))
	exits[0].Signature, err = signing.ComputeDomainAndSign(state, time.CurrentEpoch(state), exits[0].Exit, params.BeaconConfig().DomainVoluntaryExit, priv)
	require.NoError(t, err)

	b := util.NewBeaconBlockZond()
	b.Block = &qrysmpb.BeaconBlockZond{
		Body: &qrysmpb.BeaconBlockBodyZond{
			VoluntaryExits: exits,
		},
	}

	newState, err := blocks.ProcessVoluntaryExits(context.Background(), state, b.Block.Body.VoluntaryExits)
	require.NoError(t, err, "Could not process exits")
	newRegistry := newState.Validators()
	if newRegistry[0].ExitEpoch != helpers.ActivationExitEpoch(primitives.Epoch(state.Slot()/params.BeaconConfig().SlotsPerEpoch)) {
		t.Errorf("Expected validator exit epoch to be %d, got %d",
			helpers.ActivationExitEpoch(primitives.Epoch(state.Slot()/params.BeaconConfig().SlotsPerEpoch)), newRegistry[0].ExitEpoch)
	}
}

func TestVerifyExitAndSignature(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() (*qrysmpb.Validator, *qrysmpb.SignedVoluntaryExit, state.ReadOnlyBeaconState, error)
		wantErr string
	}{
		{
			name: "Empty Exit",
			setup: func() (*qrysmpb.Validator, *qrysmpb.SignedVoluntaryExit, state.ReadOnlyBeaconState, error) {
				fork := &qrysmpb.Fork{
					PreviousVersion: params.BeaconConfig().GenesisForkVersion,
					CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
					Epoch:           0,
				}
				genesisRoot := [32]byte{'a'}

				st := &qrysmpb.BeaconStateZond{
					Slot:                  0,
					Fork:                  fork,
					GenesisValidatorsRoot: genesisRoot[:],
				}

				s, err := state_native.InitializeFromProtoUnsafeZond(st)
				if err != nil {
					return nil, nil, nil, err
				}
				return &qrysmpb.Validator{}, &qrysmpb.SignedVoluntaryExit{}, s, nil
			},
			wantErr: "nil exit",
		},
		{
			name: "Happy Path",
			setup: func() (*qrysmpb.Validator, *qrysmpb.SignedVoluntaryExit, state.ReadOnlyBeaconState, error) {
				fork := &qrysmpb.Fork{
					PreviousVersion: params.BeaconConfig().GenesisForkVersion,
					CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
					Epoch:           0,
				}
				signedExit := &qrysmpb.SignedVoluntaryExit{
					Exit: &qrysmpb.VoluntaryExit{
						Epoch:          2,
						ValidatorIndex: 0,
					},
				}
				bs, keys := util.DeterministicGenesisStateZond(t, 1)
				validator := bs.Validators()[0]
				validator.ActivationEpoch = 1
				err := bs.UpdateValidatorAtIndex(0, validator)
				require.NoError(t, err)
				sb, err := signing.ComputeDomainAndSign(bs, signedExit.Exit.Epoch, signedExit.Exit, params.BeaconConfig().DomainVoluntaryExit, keys[0])
				require.NoError(t, err)
				sig, err := ml_dsa_87.SignatureFromBytes(sb)
				require.NoError(t, err)
				signedExit.Signature = sig.Marshal()
				if err := bs.SetFork(fork); err != nil {
					return nil, nil, nil, err
				}
				if err := bs.SetSlot((params.BeaconConfig().SlotsPerEpoch * 2) + 1); err != nil {
					return nil, nil, nil, err
				}
				return validator, signedExit, bs, nil
			},
		},
		{
			name: "bad signature",
			setup: func() (*qrysmpb.Validator, *qrysmpb.SignedVoluntaryExit, state.ReadOnlyBeaconState, error) {
				fork := &qrysmpb.Fork{
					PreviousVersion: params.BeaconConfig().GenesisForkVersion,
					CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
					Epoch:           0,
				}
				signedExit := &qrysmpb.SignedVoluntaryExit{
					Exit: &qrysmpb.VoluntaryExit{
						Epoch:          2,
						ValidatorIndex: 0,
					},
				}
				bs, keys := util.DeterministicGenesisStateZond(t, 1)
				validator := bs.Validators()[0]
				validator.ActivationEpoch = 1

				sb, err := signing.ComputeDomainAndSign(bs, signedExit.Exit.Epoch, signedExit.Exit, params.BeaconConfig().DomainVoluntaryExit, keys[0])
				require.NoError(t, err)
				sig, err := ml_dsa_87.SignatureFromBytes(sb)
				require.NoError(t, err)
				signedExit.Signature = sig.Marshal()
				if err := bs.SetFork(fork); err != nil {
					return nil, nil, nil, err
				}
				if err := bs.SetSlot((params.BeaconConfig().SlotsPerEpoch * 2) + 1); err != nil {
					return nil, nil, nil, err
				}
				// use wrong genesis root and don't update validator
				genesisRoot := [32]byte{'a'}
				if err := bs.SetGenesisValidatorsRoot(genesisRoot[:]); err != nil {
					return nil, nil, nil, err
				}
				return validator, signedExit, bs, nil
			},
			wantErr: "signature did not verify",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := params.BeaconConfig().ShardCommitteePeriod
			params.BeaconConfig().ShardCommitteePeriod = 0
			validator, signedExit, st, err := tt.setup()
			require.NoError(t, err)
			rvalidator, err := state_native.NewValidator(validator)
			require.NoError(t, err)
			err = blocks.VerifyExitAndSignature(
				rvalidator,
				st,
				signedExit,
			)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, tt.wantErr, err)
			}
			params.BeaconConfig().ShardCommitteePeriod = c // prevent contamination
		})
	}
}
