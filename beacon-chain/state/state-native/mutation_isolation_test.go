package state_native_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/epoch"
	"github.com/theQRL/qrysm/beacon-chain/state"
	statenative "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/features"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func isolationState(t *testing.T) state.BeaconState {
	t.Helper()
	if fieldparams.Preset == params.MinimalName {
		params.SetupTestConfigCleanup(t)
		params.OverrideBeaconConfig(params.MinimalSpecConfig())
	}
	st, err := util.NewBeaconStateZond(func(pb *qrysmpb.BeaconStateZond) error {
		for i := byte(1); i <= 2; i++ {
			key := make([]byte, fieldparams.MLDSA87PubkeyLength)
			key[0] = i
			pb.Validators = append(pb.Validators, &qrysmpb.Validator{
				PublicKey: key, WithdrawalRecipient: make([]byte, 64), RandaoCommitment: make([]byte, 32),
			})
		}
		pb.Balances = []uint64{1, 2}
		pb.InactivityScores = []uint64{0, 0}
		pb.CurrentEpochParticipation = []byte{0, 0}
		pb.PreviousEpochParticipation = []byte{0, 0}
		return nil
	})
	require.NoError(t, err)
	return st
}

func requireConsistentStateRoot(t *testing.T, st state.BeaconState) {
	t.Helper()
	pb := st.ToProto().(*qrysmpb.BeaconStateZond)
	var want [32]byte
	var err error
	if fieldparams.Preset == params.MinimalName {
		// The checked-in protobuf SSZ code uses mainnet sizes. A fresh native
		// state still provides an uncached reference for minimal builds.
		fresh, initErr := statenative.InitializeFromProtoZond(pb)
		require.NoError(t, initErr)
		want, err = fresh.HashTreeRoot(context.Background())
	} else {
		want, err = pb.HashTreeRoot()
	}
	require.NoError(t, err)
	got, err := st.HashTreeRoot(context.Background())
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestRootReplacement_CopyIsolation(t *testing.T) {
	fields := []struct {
		name   string
		length int
		set    func(state.BeaconState, [][]byte) error
		get    func(state.BeaconState, uint64) ([]byte, error)
		update func(state.BeaconState, uint64, [32]byte) error
	}{
		{"block roots", fieldparams.BlockRootsLength, state.BeaconState.SetBlockRoots, state.BeaconState.BlockRootAtIndex, state.BeaconState.UpdateBlockRootAtIndex},
		{"state roots", fieldparams.StateRootsLength, state.BeaconState.SetStateRoots, state.BeaconState.StateRootAtIndex, state.BeaconState.UpdateStateRootAtIndex},
		{"randao mixes", fieldparams.RandaoMixesLength, state.BeaconState.SetRandaoMixes, state.BeaconState.RandaoMixAtIndex, state.BeaconState.UpdateRandaoMixesAtIndex},
	}
	for _, experimental := range []bool{false, true} {
		t.Run(fmt.Sprintf("experimental=%v", experimental), func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{EnableExperimentalState: experimental})
			defer reset()
			for _, field := range fields {
				for _, length := range []int{0, field.length - 1, field.length, field.length + 1} {
					t.Run(fmt.Sprintf("%s/length=%d", field.name, length), func(t *testing.T) {
						st := isolationState(t)
						require.NoError(t, field.update(st, 0, [32]byte{1}))
						requireConsistentStateRoot(t, st)
						snapshot := st.Copy()
						roots := make([][]byte, length)
						for i := range roots {
							roots[i] = make([]byte, 32)
							roots[i][0] = 2
						}
						func() {
							defer func() {
								if r := recover(); r != nil {
									t.Errorf("root replacement panicked: %v", r)
								}
							}()
							if err := field.set(st, roots); length == field.length {
								require.NoError(t, err)
							} else if err == nil {
								t.Error("expected an error for an invalid root count")
							}
						}()
						want := byte(1)
						if length == field.length {
							want = 2
							roots[0][0] = 9 // Successful replacement must own its input.
						}
						root, err := field.get(st, 0)
						require.NoError(t, err)
						if root[0] != want {
							t.Errorf("replacement left root %d, want %d", root[0], want)
						}
						// Failed replacement must preserve shared ownership as well as values.
						require.NoError(t, field.update(st, 0, [32]byte{3}))
						root, err = field.get(snapshot, 0)
						require.NoError(t, err)
						if root[0] != 1 {
							t.Errorf("subsequent write changed the snapshot root to %d", root[0])
						}
						requireConsistentStateRoot(t, snapshot)
						requireConsistentStateRoot(t, st)
					})
				}
			}
		})
	}
}

func TestApplyToEveryValidator_CopyIsolation(t *testing.T) {
	for _, experimental := range []bool{false, true} {
		t.Run(fmt.Sprintf("experimental=%v", experimental), func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{EnableExperimentalState: experimental})
			defer reset()
			for _, outcome := range []string{"success", "unchanged", "error", "early error with replacement"} {
				t.Run(outcome, func(t *testing.T) {
					original := isolationState(t)
					requireConsistentStateRoot(t, original)
					st := original.Copy()
					var during state.BeaconState
					callbackErr := errors.New("callback failed")
					err := st.ApplyToEveryValidator(func(i int, val *qrysmpb.Validator) (bool, *qrysmpb.Validator, error) {
						if i == 0 {
							during = st.Copy() // Callbacks may reenter state methods.
						}
						if outcome == "early error with replacement" {
							if i > 0 {
								return false, nil, callbackErr
							}
							val = qrysmpb.CopyValidator(val)
						}
						val.ExitEpoch = 42
						if outcome == "error" {
							return false, nil, callbackErr
						}
						return outcome != "unchanged", val, nil
					})
					if outcome == "error" || outcome == "early error with replacement" {
						require.ErrorIs(t, err, callbackErr)
					} else {
						require.NoError(t, err)
					}
					want := primitives.Epoch(0)
					if outcome == "success" || outcome == "early error with replacement" {
						want = 42
					}
					val, err := st.ValidatorAtIndex(0)
					require.NoError(t, err)
					require.Equal(t, want, val.ExitEpoch, "successful updates before an error must remain applied")
					// A subsequent write must also respect the original shared ownership.
					val.ExitEpoch = 99
					require.NoError(t, st.UpdateValidatorAtIndex(0, val))
					for _, snapshot := range []state.BeaconState{original, during} {
						v, err := snapshot.ValidatorAtIndex(0)
						require.NoError(t, err)
						require.Equal(t, primitives.Epoch(0), v.ExitEpoch, "callback changed an earlier state copy")
						requireConsistentStateRoot(t, snapshot)
					}
					requireConsistentStateRoot(t, st)
				})
			}
		})
	}
}

func TestApplyToEveryValidator_NilEntry(t *testing.T) {
	for _, experimental := range []bool{false, true} {
		t.Run(fmt.Sprintf("experimental=%v", experimental), func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{EnableExperimentalState: experimental})
			defer reset()
			for _, nilEntry := range []bool{false, true} {
				t.Run(fmt.Sprintf("nil=%v", nilEntry), func(t *testing.T) {
					defer func() {
						if r := recover(); r != nil {
							t.Errorf("validator iteration panicked: %v", r)
						}
					}()
					original := isolationState(t)
					val, err := original.ValidatorAtIndex(0)
					require.NoError(t, err)
					val.EffectiveBalance = params.BeaconConfig().MaxEffectiveBalance
					require.NoError(t, original.UpdateValidatorAtIndex(0, val))
					requireConsistentStateRoot(t, original)
					st := original.Copy()
					if nilEntry {
						require.NoError(t, st.UpdateValidatorAtIndex(1, nil))
					}
					_, err = epoch.ProcessEffectiveBalanceUpdates(st)
					if nilEntry {
						require.ErrorContains(t, "validator 1 is nil in state", err)
						// Repair the malformed entry so hashing can check the completed prefix.
						replacement, err := original.ValidatorAtIndex(1)
						require.NoError(t, err)
						require.NoError(t, st.UpdateValidatorAtIndex(1, replacement))
					} else {
						require.NoError(t, err)
					}
					val, err = st.ValidatorAtIndex(0)
					require.NoError(t, err)
					require.Equal(t, uint64(0), val.EffectiveBalance, "completed update was lost")
					val, err = original.ValidatorAtIndex(0)
					require.NoError(t, err)
					require.Equal(t, params.BeaconConfig().MaxEffectiveBalance, val.EffectiveBalance, "snapshot was changed")
					requireConsistentStateRoot(t, original)
					requireConsistentStateRoot(t, st)
				})
			}
		})
	}
}

func TestAppendValidator_NilPreservesState(t *testing.T) {
	for _, experimental := range []bool{false, true} {
		t.Run(fmt.Sprintf("experimental=%v", experimental), func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{EnableExperimentalState: experimental})
			defer reset()
			for _, nilEntry := range []bool{false, true} {
				t.Run(fmt.Sprintf("nil=%v", nilEntry), func(t *testing.T) {
					original := isolationState(t)
					requireConsistentStateRoot(t, original)
					st := original.Copy()
					val, err := st.ValidatorAtIndex(0)
					require.NoError(t, err)
					val.PublicKey[0] = 3
					if nilEntry {
						func() {
							defer func() {
								if r := recover(); r != nil {
									t.Errorf("nil validator append panicked: %v", r)
								}
							}()
							require.ErrorContains(t, "nil validator", st.AppendValidator(nil))
						}()
						require.Equal(t, original.NumValidators(), st.NumValidators(), "failed append changed registry length")
						require.DeepSSZEqual(t, original.ToProto(), st.ToProto())
						requireConsistentStateRoot(t, st)
					}
					// A subsequent valid append must behave exactly like the healthy control.
					require.NoError(t, st.AppendValidator(val))
					require.Equal(t, 3, st.NumValidators())
					key := st.PubkeyAtIndex(2)
					idx, ok := st.ValidatorIndexByPubkey(key)
					require.Equal(t, true, ok)
					require.Equal(t, primitives.ValidatorIndex(2), idx)
					_, ok = original.ValidatorIndexByPubkey(key)
					require.Equal(t, false, ok, "append changed snapshot lookup")
					require.Equal(t, 2, original.NumValidators())
					requireConsistentStateRoot(t, original)
					requireConsistentStateRoot(t, st)
				})
			}
		})
	}
}

func TestParticipationMutation_CopyIsolation(t *testing.T) {
	for _, previous := range []bool{false, true} {
		for _, shared := range []bool{false, true} {
			for _, fail := range []bool{false, true} {
				t.Run(fmt.Sprintf("previous=%v/shared=%v/error=%v", previous, shared, fail), func(t *testing.T) {
					st := isolationState(t)
					set, get, modify := st.SetCurrentParticipationBits, state.BeaconState.CurrentEpochParticipation, st.ModifyCurrentParticipationBits
					if previous {
						set, get, modify = st.SetPreviousParticipationBits, state.BeaconState.PreviousEpochParticipation, st.ModifyPreviousParticipationBits
					}
					require.NoError(t, set([]byte{0, 0}))
					requireConsistentStateRoot(t, st)
					var original, during state.BeaconState
					if shared {
						original = st.Copy()
					}
					callbackErr := errors.New("callback failed")
					err := modify(func(bits []byte) ([]byte, error) {
						if !shared {
							during = st.Copy()
						}
						bits[0] = 1
						if fail {
							return nil, callbackErr
						}
						return bits, nil
					})
					if fail {
						require.ErrorIs(t, err, callbackErr)
					} else {
						require.NoError(t, err)
					}
					bits, err := get(st)
					require.NoError(t, err)
					want := byte(1)
					if fail {
						want = 0
					}
					require.Equal(t, want, bits[0])
					requireConsistentStateRoot(t, st)
					require.NoError(t, modify(func(bits []byte) ([]byte, error) { bits[0] = 2; return bits, nil }))
					for _, snapshot := range []state.BeaconState{original, during} {
						if snapshot == nil {
							continue
						}
						bits, err := get(snapshot)
						require.NoError(t, err)
						require.DeepEqual(t, []byte{0, 0}, bits)
						requireConsistentStateRoot(t, snapshot)
					}
					requireConsistentStateRoot(t, st)
				})
			}
		}
	}
}

func TestValidatorIndexByPubkey_CopyIsolation(t *testing.T) {
	for _, experimental := range []bool{false, true} {
		t.Run(fmt.Sprintf("experimental=%v", experimental), func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{EnableExperimentalState: experimental})
			defer reset()
			a := isolationState(t)
			b := a.Copy()
			va, vb := a.Validators()[0], b.Validators()[0]
			va.PublicKey[0], vb.PublicKey[0] = 3, 4
			require.NoError(t, a.AppendValidator(va))
			require.NoError(t, b.AppendValidator(vb))
			keyA, keyB := a.PubkeyAtIndex(2), b.PubkeyAtIndex(2)
			for _, tc := range []struct {
				st              state.BeaconState
				present, absent [fieldparams.MLDSA87PubkeyLength]byte
			}{{a, keyA, keyB}, {b, keyB, keyA}} {
				idx, ok := tc.st.ValidatorIndexByPubkey(tc.present)
				require.Equal(t, true, ok)
				require.Equal(t, primitives.ValidatorIndex(2), idx)
				_, ok = tc.st.ValidatorIndexByPubkey(tc.absent)
				require.Equal(t, false, ok, "pubkey from another branch leaked into this state")
			}
			// The same key can occur at different indices on competing branches.
			require.NoError(t, b.AppendValidator(qrysmpb.CopyValidator(va)))
			idx, ok := a.ValidatorIndexByPubkey(keyA)
			require.Equal(t, true, ok)
			require.Equal(t, primitives.ValidatorIndex(2), idx)
			idx, ok = b.ValidatorIndexByPubkey(keyA)
			require.Equal(t, true, ok)
			require.Equal(t, primitives.ValidatorIndex(3), idx)
		})
	}
}

func TestGenesisValidatorsRoot_CopyIsolation(t *testing.T) {
	st := isolationState(t)
	requireConsistentStateRoot(t, st)
	want := append([]byte(nil), st.GenesisValidatorsRoot()...)
	returned := st.GenesisValidatorsRoot()
	returned[0] ^= 0xff
	require.DeepEqual(t, want, st.GenesisValidatorsRoot())
	requireConsistentStateRoot(t, st)
}

func TestStateProof_CopyIsolation(t *testing.T) {
	for _, method := range []string{"current", "next", "finalized"} {
		t.Run(method, func(t *testing.T) {
			st := isolationState(t)
			proofFn := st.CurrentSyncCommitteeProof
			if method == "next" {
				proofFn = st.NextSyncCommitteeProof
			} else if method == "finalized" {
				proofFn = st.FinalizedRootProof
			}
			proof, err := proofFn(context.Background())
			require.NoError(t, err)
			want := make([][]byte, len(proof))
			for i := range proof {
				want[i] = append([]byte(nil), proof[i]...)
				proof[i][0] ^= 0xff
			}
			got, err := proofFn(context.Background())
			require.NoError(t, err)
			require.DeepEqual(t, want, got, "returned proof aliases cached Merkle layers")
			require.NoError(t, st.SetSlot(1))
			requireConsistentStateRoot(t, st)
		})
	}
}

func TestConcurrentStateAccess(t *testing.T) {
	for _, experimental := range []bool{false, true} {
		t.Run(fmt.Sprintf("experimental=%v", experimental), func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{EnableExperimentalState: experimental})
			defer reset()
			st := isolationState(t)
			var wg sync.WaitGroup
			wg.Add(1)
			defer wg.Wait()
			go func() {
				defer wg.Done()
				for range 20 {
					// Use fresh setter inputs, so any race is between state methods.
					_ = st.SetFork(&qrysmpb.Fork{})
					_ = st.SetLatestBlockHeader(&qrysmpb.BeaconBlockHeader{})
					_ = st.SetExecutionData(&qrysmpb.ExecutionData{})
					_ = st.SetExecutionDataVotes(nil)
					_ = st.SetJustificationBits(nil)
					_ = st.SetPreviousJustifiedCheckpoint(&qrysmpb.Checkpoint{})
					_ = st.SetCurrentJustifiedCheckpoint(&qrysmpb.Checkpoint{})
					_ = st.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{})
					_ = st.SetHistoricalRoots(nil)
					_ = st.AppendHistoricalSummaries(&qrysmpb.HistoricalSummary{})
					_ = st.SetCurrentParticipationBits([]byte{0, 0})
					_ = st.SetPreviousParticipationBits([]byte{0, 0})
					_ = st.SetSlashings([]uint64{0, 0})
					_ = st.SetValidators(nil)
					_ = st.SetBalances([]uint64{0, 0})
					_ = st.SetInactivityScores([]uint64{0, 0})
					runtime.Gosched()
				}
			}()
			for range 20 {
				_ = st.Fork()
				_ = st.LatestBlockHeader()
				_ = st.ExecutionData()
				_ = st.ExecutionDataVotes()
				_ = st.JustificationBits()
				_ = st.PreviousJustifiedCheckpoint()
				_ = st.CurrentJustifiedCheckpoint()
				_ = st.FinalizedCheckpoint()
				_ = st.FinalizedCheckpointEpoch()
				_ = st.MatchCurrentJustifiedCheckpoint(&qrysmpb.Checkpoint{})
				_ = st.MatchPreviousJustifiedCheckpoint(&qrysmpb.Checkpoint{})
				_, _ = st.HistoricalRoots()
				_, _ = st.HistoricalSummaries()
				_, _ = st.CurrentEpochParticipation()
				_, _ = st.PreviousEpochParticipation()
				_ = st.Slashings()
				_, _ = st.ValidatorIndexByPubkey([fieldparams.MLDSA87PubkeyLength]byte{})
				_, err := st.MarshalJSON()
				require.NoError(t, err)
				_ = st.UpdateValidatorAtIndex(0, &qrysmpb.Validator{}) // Registry can be empty.
				require.NoError(t, st.AppendValidator(&qrysmpb.Validator{}))
				require.NoError(t, st.UpdateBalancesAtIndex(0, 1))
				require.NoError(t, st.AppendBalance(1))
				require.NoError(t, st.AppendInactivityScore(1))
				require.NoError(t, st.UpdateSlashingsAtIndex(0, 1))
				runtime.Gosched()
			}
			wg.Wait()
		})
	}
}

func TestConcurrentStateCopies_HashConsistency(t *testing.T) {
	for _, experimental := range []bool{false, true} {
		t.Run(fmt.Sprintf("experimental=%v", experimental), func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{EnableExperimentalState: experimental})
			defer reset()
			st := isolationState(t)
			requireConsistentStateRoot(t, st)
			done := make(chan error, 1)
			var wg sync.WaitGroup
			wg.Add(1)
			defer wg.Wait()
			go func() {
				defer wg.Done()
				update := func() error {
					for i := range 16 {
						val, err := st.ValidatorAtIndex(0)
						if err != nil {
							return err
						}
						val.ExitEpoch = primitives.Epoch(i)
						for _, mutate := range []func() error{
							func() error { return st.UpdateStateRootAtIndex(0, [32]byte{byte(i)}) },
							func() error { return st.UpdateRandaoMixesAtIndex(0, [32]byte{byte(i)}) },
							func() error { return st.UpdateValidatorAtIndex(0, val) },
							func() error { return st.UpdateBalancesAtIndex(0, uint64(i)) },
							func() error { return st.AppendBalance(uint64(i)) },
							func() error { return st.AppendInactivityScore(uint64(i)) },
						} {
							if err := mutate(); err != nil {
								return err
							}
							runtime.Gosched()
						}
					}
					return nil
				}
				done <- update()
			}()
			for range 16 {
				snapshot := st.Copy()
				requireConsistentStateRoot(t, snapshot)
				require.NoError(t, snapshot.UpdateBalancesAtIndex(0, 100))
				requireConsistentStateRoot(t, snapshot)
			}
			require.NoError(t, <-done)
			requireConsistentStateRoot(t, st)
		})
	}
}
