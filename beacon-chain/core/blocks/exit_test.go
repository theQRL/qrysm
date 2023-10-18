package blocks_test

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/time"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/crypto/bls"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestProcessVoluntaryExits_NotActiveLongEnoughToExit(t *testing.T) {
	exits := []*zondpb.SignedVoluntaryExit{
		{
			Exit: &zondpb.VoluntaryExit{
				ValidatorIndex: 0,
				Epoch:          0,
			},
		},
	}
	registry := []*zondpb.Validator{
		{
			ExitEpoch: params.BeaconConfig().FarFutureEpoch,
		},
	}
	state, err := state_native.InitializeFromProtoPhase0(&zondpb.BeaconState{
		Validators: registry,
		Slot:       10,
	})
	require.NoError(t, err)
	b := util.NewBeaconBlock()
	b.Block = &zondpb.BeaconBlock{
		Body: &zondpb.BeaconBlockBody{
			VoluntaryExits: exits,
		},
	}

	want := "validator has not been active long enough to exit"
	_, err = blocks.ProcessVoluntaryExits(context.Background(), state, b.Block.Body.VoluntaryExits)
	assert.ErrorContains(t, want, err)
}

func TestProcessVoluntaryExits_ExitAlreadySubmitted(t *testing.T) {
	exits := []*zondpb.SignedVoluntaryExit{
		{
			Exit: &zondpb.VoluntaryExit{
				Epoch: 10,
			},
		},
	}
	registry := []*zondpb.Validator{
		{
			ExitEpoch: 10,
		},
	}
	state, err := state_native.InitializeFromProtoPhase0(&zondpb.BeaconState{
		Validators: registry,
		Slot:       0,
	})
	require.NoError(t, err)
	b := util.NewBeaconBlock()
	b.Block = &zondpb.BeaconBlock{
		Body: &zondpb.BeaconBlockBody{
			VoluntaryExits: exits,
		},
	}

	want := "validator with index 0 has already submitted an exit, which will take place at epoch: 10"
	_, err = blocks.ProcessVoluntaryExits(context.Background(), state, b.Block.Body.VoluntaryExits)
	assert.ErrorContains(t, want, err)
}

func TestProcessVoluntaryExits_AppliesCorrectStatus(t *testing.T) {
	exits := []*zondpb.SignedVoluntaryExit{
		{
			Exit: &zondpb.VoluntaryExit{
				ValidatorIndex: 0,
				Epoch:          0,
			},
		},
	}
	registry := []*zondpb.Validator{
		{
			ExitEpoch:       params.BeaconConfig().FarFutureEpoch,
			ActivationEpoch: 0,
		},
	}
	state, err := state_native.InitializeFromProtoPhase0(&zondpb.BeaconState{
		Validators: registry,
		Fork: &zondpb.Fork{
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
		},
		Slot: params.BeaconConfig().SlotsPerEpoch * 5,
	})
	require.NoError(t, err)
	err = state.SetSlot(state.Slot() + params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().ShardCommitteePeriod)))
	require.NoError(t, err)

	priv, err := bls.RandKey()
	require.NoError(t, err)

	val, err := state.ValidatorAtIndex(0)
	require.NoError(t, err)
	val.PublicKey = priv.PublicKey().Marshal()
	require.NoError(t, state.UpdateValidatorAtIndex(0, val))
	exits[0].Signature, err = signing.ComputeDomainAndSign(state, time.CurrentEpoch(state), exits[0].Exit, params.BeaconConfig().DomainVoluntaryExit, priv)
	require.NoError(t, err)

	b := util.NewBeaconBlock()
	b.Block = &zondpb.BeaconBlock{
		Body: &zondpb.BeaconBlockBody{
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
	type args struct {
		currentSlot primitives.Slot
	}

	tests := []struct {
		name    string
		args    args
		setup   func() (*zondpb.Validator, *zondpb.SignedVoluntaryExit, *zondpb.Fork, []byte, error)
		wantErr string
	}{
		{
			name: "Empty Exit",
			args: args{
				currentSlot: 0,
			},
			setup: func() (*zondpb.Validator, *zondpb.SignedVoluntaryExit, *zondpb.Fork, []byte, error) {
				fork := &zondpb.Fork{
					PreviousVersion: params.BeaconConfig().GenesisForkVersion,
					CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
					Epoch:           0,
				}
				genesisRoot := [32]byte{'a'}
				return &zondpb.Validator{}, &zondpb.SignedVoluntaryExit{}, fork, genesisRoot[:], nil
			},
			wantErr: "nil exit",
		},
		{
			name: "Happy Path",
			args: args{
				currentSlot: (params.BeaconConfig().SlotsPerEpoch * 2) + 1,
			},
			setup: func() (*zondpb.Validator, *zondpb.SignedVoluntaryExit, *zondpb.Fork, []byte, error) {
				fork := &zondpb.Fork{
					PreviousVersion: params.BeaconConfig().GenesisForkVersion,
					CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
					Epoch:           0,
				}
				signedExit := &zondpb.SignedVoluntaryExit{
					Exit: &zondpb.VoluntaryExit{
						Epoch:          2,
						ValidatorIndex: 0,
					},
				}
				bs, keys := util.DeterministicGenesisState(t, 1)
				validator := bs.Validators()[0]
				validator.ActivationEpoch = 1
				err := bs.UpdateValidatorAtIndex(0, validator)
				require.NoError(t, err)
				sb, err := signing.ComputeDomainAndSign(bs, signedExit.Exit.Epoch, signedExit.Exit, params.BeaconConfig().DomainVoluntaryExit, keys[0])
				require.NoError(t, err)
				sig, err := bls.SignatureFromBytes(sb)
				require.NoError(t, err)
				signedExit.Signature = sig.Marshal()
				return validator, signedExit, fork, bs.GenesisValidatorsRoot(), nil
			},
		},
		{
			name: "bad signature",
			args: args{
				currentSlot: (params.BeaconConfig().SlotsPerEpoch * 2) + 1,
			},
			setup: func() (*zondpb.Validator, *zondpb.SignedVoluntaryExit, *zondpb.Fork, []byte, error) {
				fork := &zondpb.Fork{
					PreviousVersion: params.BeaconConfig().GenesisForkVersion,
					CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
					Epoch:           0,
				}
				signedExit := &zondpb.SignedVoluntaryExit{
					Exit: &zondpb.VoluntaryExit{
						Epoch:          2,
						ValidatorIndex: 0,
					},
				}
				bs, keys := util.DeterministicGenesisState(t, 1)
				validator := bs.Validators()[0]
				validator.ActivationEpoch = 1

				sb, err := signing.ComputeDomainAndSign(bs, signedExit.Exit.Epoch, signedExit.Exit, params.BeaconConfig().DomainVoluntaryExit, keys[0])
				require.NoError(t, err)
				sig, err := bls.SignatureFromBytes(sb)
				require.NoError(t, err)
				signedExit.Signature = sig.Marshal()
				genesisRoot := [32]byte{'a'}
				// use wrong genesis root and don't update validator
				return validator, signedExit, fork, genesisRoot[:], nil
			},
			wantErr: "signature did not verify",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := params.BeaconConfig().ShardCommitteePeriod
			params.BeaconConfig().ShardCommitteePeriod = 0
			validator, signedExit, fork, genesisRoot, err := tt.setup()
			require.NoError(t, err)
			rvalidator, err := state_native.NewValidator(validator)
			require.NoError(t, err)
			err = blocks.VerifyExitAndSignature(
				rvalidator,
				tt.args.currentSlot,
				fork,
				signedExit,
				genesisRoot,
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
