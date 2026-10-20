package helpers

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestCommitteeCache_StateIsolation(t *testing.T) {
	ctx := context.Background()
	cfg := params.BeaconConfig()
	const epoch = primitives.Epoch(2)
	for _, change := range []string{"exit", "activation", "different members with equal counts"} {
		for _, reverse := range []bool{false, true} {
			for _, lookup := range []string{"indices", "count", "committee from state", "committee from indices"} {
				t.Run(fmt.Sprintf("%s/reverse=%t/%s", change, reverse, lookup), func(t *testing.T) {
					ClearCache()
					t.Cleanup(ClearCache)
					a, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
						RandaoMixes: make([][]byte, cfg.EpochsPerHistoricalVector),
					})
					require.NoError(t, err)
					vals := make([]*qrysmpb.Validator, 256)
					for i := range vals {
						vals[i] = &qrysmpb.Validator{ExitEpoch: cfg.FarFutureEpoch}
					}
					if change == "activation" {
						vals[0].ActivationEpoch = epoch + 1
					}
					if change == "different members with equal counts" {
						vals[1].ExitEpoch = epoch
					}
					require.NoError(t, a.SetValidators(vals))
					// Query next epoch, as epoch-boundary cache warming does.
					require.NoError(t, a.SetSlot(primitives.Slot(epoch)*cfg.SlotsPerEpoch-1))
					b := a.Copy()
					val, err := b.ValidatorAtIndex(0)
					require.NoError(t, err)
					if change == "activation" {
						val.ActivationEpoch = epoch
					} else {
						val.ExitEpoch = epoch
					}
					require.NoError(t, b.UpdateValidatorAtIndex(0, val))
					if change == "different members with equal counts" {
						val, err := b.ValidatorAtIndex(1)
						require.NoError(t, err)
						val.ExitEpoch = cfg.FarFutureEpoch
						require.NoError(t, b.UpdateValidatorAtIndex(1, val))
					}
					if reverse {
						a, b = b, a
					}
					seed, err := Seed(a, epoch, cfg.DomainBeaconAttester)
					require.NoError(t, err)
					otherSeed, err := Seed(b, epoch, cfg.DomainBeaconAttester)
					require.NoError(t, err)
					require.Equal(t, seed, otherSeed)

					// Alternate branches without clearing the cache. Compare each
					// lookup with membership read directly from that branch.
					require.NoError(t, UpdateCommitteeCache(ctx, a, epoch))
					for _, target := range []state.BeaconState{b, a, b} {
						var want []primitives.ValidatorIndex
						for i, validator := range target.Validators() {
							if validator.ActivationEpoch <= epoch && epoch < validator.ExitEpoch {
								want = append(want, primitives.ValidatorIndex(i))
							}
						}
						switch lookup {
						case "indices":
							got, err := ActiveValidatorIndices(ctx, target, epoch)
							require.NoError(t, err)
							require.DeepEqual(t, want, got)
						case "count":
							got, err := ActiveValidatorCount(ctx, target, epoch)
							require.NoError(t, err)
							require.Equal(t, uint64(len(want)), got)
						default:
							perSlot := SlotCommitteeCount(uint64(len(want)))
							for offset := primitives.Slot(0); offset < cfg.SlotsPerEpoch; offset++ {
								for index := uint64(0); index < perSlot; index++ {
									// computeCommittee bypasses the shared cache.
									expected, err := computeCommittee(want, seed, uint64(offset)*perSlot+index, uint64(cfg.SlotsPerEpoch)*perSlot)
									require.NoError(t, err)
									slot := primitives.Slot(epoch)*cfg.SlotsPerEpoch + offset
									var got []primitives.ValidatorIndex
									if lookup == "committee from state" {
										got, err = BeaconCommitteeFromState(ctx, target, slot, primitives.CommitteeIndex(index))
									} else {
										got, err = BeaconCommittee(ctx, want, seed, slot, primitives.CommitteeIndex(index))
									}
									require.NoError(t, err)
									require.DeepEqual(t, expected, got, "slot %d committee %d", slot, index)
								}
							}
						}
					}
				})
			}
		}
	}
}
