package blockchain

import (
	"bytes"
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	dbTest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
)

func TestService_HeadAggregatorSelectionSeed(t *testing.T) {
	st, _ := util.DeterministicGenesisStateZond(t, params.BeaconConfig().SyncCommitteeSize)
	c := &Service{head: &head{state: st}}
	ctx := context.Background()
	// The last slot before a period transition uses the message epoch's seed,
	// even though its sync membership is drawn from the next period.
	slot := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)) - 1
	require.NoError(t, st.SetSlot(slot))
	epoch := slots.ToEpoch(slot)
	expected, err := helpers.AggregatorSelectionSeed(st, epoch)
	require.NoError(t, err)
	actual, err := c.HeadAggregatorSelectionSeed(ctx, slot)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
	nextEpochSeed, err := helpers.AggregatorSelectionSeed(st, epoch+1)
	require.NoError(t, err)
	require.NotEqual(t, nextEpochSeed, actual)

	// A replacement head at the same slot must replace the seed, even if the
	// sync committee cache still contains the state from the old branch.
	t.Cleanup(func() { syncCommitteeHeadStateCache = cache.NewSyncCommitteeHeadState() })
	require.NoError(t, syncCommitteeHeadStateCache.Put(c.head.root, slot, st))
	replacement := st.Copy()
	lookahead := (epoch + params.BeaconConfig().EpochsPerHistoricalVector - params.BeaconConfig().MinSeedLookahead - 1) % params.BeaconConfig().EpochsPerHistoricalVector
	require.NoError(t, replacement.UpdateRandaoMixesAtIndex(uint64(lookahead), [32]byte{42}))
	c.head = &head{state: replacement}
	updated, err := c.HeadAggregatorSelectionSeed(ctx, slot)
	require.NoError(t, err)
	require.NotEqual(t, actual, updated)
	expected, err = helpers.AggregatorSelectionSeed(replacement, epoch)
	require.NoError(t, err)
	require.Equal(t, expected, updated)
}

func TestService_HeadSyncCommitteeIndices(t *testing.T) {
	transition.SkipSlotCache.Disable()
	defer transition.SkipSlotCache.Enable()

	s, _ := util.DeterministicGenesisStateZond(t, params.BeaconConfig().SyncCommitteeSize)
	c := &Service{cfg: &config{BeaconDB: dbTest.SetupDB(t)}}
	c.head = &head{state: s}

	// Current period
	slot := 2*uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)*uint64(params.BeaconConfig().SlotsPerEpoch) + 1
	a, err := c.HeadSyncCommitteeIndices(context.Background(), 0, primitives.Slot(slot))
	require.NoError(t, err)

	// Current period where slot-2 across EPOCHS_PER_SYNC_COMMITTEE_PERIOD
	slot = 3*uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)*uint64(params.BeaconConfig().SlotsPerEpoch) - 2
	b, err := c.HeadSyncCommitteeIndices(context.Background(), 0, primitives.Slot(slot))
	require.NoError(t, err)
	require.DeepEqual(t, a, b)

	// Next period where slot-1 across EPOCHS_PER_SYNC_COMMITTEE_PERIOD
	slot = 3*uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)*uint64(params.BeaconConfig().SlotsPerEpoch) - 1
	b, err = c.HeadSyncCommitteeIndices(context.Background(), 0, primitives.Slot(slot))
	require.NoError(t, err)
	require.DeepNotEqual(t, a, b)
}

func TestService_headCurrentSyncCommitteeIndices(t *testing.T) {
	transition.SkipSlotCache.Disable()
	defer transition.SkipSlotCache.Enable()

	s, _ := util.DeterministicGenesisStateZond(t, params.BeaconConfig().SyncCommitteeSize)
	var pubKeys [][]byte
	for i := uint64(0); i < params.BeaconConfig().SyncCommitteeSize; i++ {
		pubKeys = append(pubKeys, bytesutil.PadTo([]byte{}, field_params.MLDSA87PubkeyLength))
	}
	syncCommittee := &qrysmpb.SyncCommittee{
		Pubkeys: pubKeys,
	}
	require.NoError(t, s.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, s.SetNextSyncCommittee(syncCommittee))

	c := &Service{cfg: &config{BeaconDB: dbTest.SetupDB(t)}}
	c.head = &head{state: s}

	// Process slot up to `EpochsPerSyncCommitteePeriod` so it can `ProcessSyncCommitteeUpdates`.
	slot := uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)*uint64(params.BeaconConfig().SlotsPerEpoch) + 1
	indices, err := c.headCurrentSyncCommitteeIndices(context.Background(), 0, primitives.Slot(slot))
	require.NoError(t, err)

	// NextSyncCommittee becomes CurrentSyncCommittee so it should be empty by default.
	require.Equal(t, 0, len(indices))
}

func TestService_headNextSyncCommitteeIndices(t *testing.T) {
	transition.SkipSlotCache.Disable()
	defer transition.SkipSlotCache.Enable()

	s, _ := util.DeterministicGenesisStateZond(t, params.BeaconConfig().SyncCommitteeSize)
	var pubKeys [][]byte
	for i := uint64(0); i < params.BeaconConfig().SyncCommitteeSize; i++ {
		pubKeys = append(pubKeys, bytesutil.PadTo([]byte{}, field_params.MLDSA87PubkeyLength))
	}
	syncCommittee := &qrysmpb.SyncCommittee{
		Pubkeys: pubKeys,
	}
	require.NoError(t, s.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, s.SetNextSyncCommittee(syncCommittee))

	c := &Service{cfg: &config{BeaconDB: dbTest.SetupDB(t)}}
	c.head = &head{state: s}

	// Process slot up to `EpochsPerSyncCommitteePeriod` so it can `ProcessSyncCommitteeUpdates`.
	slot := uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)*uint64(params.BeaconConfig().SlotsPerEpoch) + 1
	indices, err := c.headNextSyncCommitteeIndices(context.Background(), 0, primitives.Slot(slot))
	require.NoError(t, err)

	// NextSyncCommittee should be be empty after `ProcessSyncCommitteeUpdates`. Validator should get indices.
	require.NotEqual(t, 0, len(indices))
}

func TestService_HeadSyncCommitteePubKeys(t *testing.T) {
	s, _ := util.DeterministicGenesisStateZond(t, params.BeaconConfig().TargetCommitteeSize)
	c := &Service{cfg: &config{BeaconDB: dbTest.SetupDB(t)}}
	c.head = &head{state: s}

	// Process slot up to 2 * `EpochsPerSyncCommitteePeriod` so it can run `ProcessSyncCommitteeUpdates` twice.
	slot := uint64(2*params.BeaconConfig().EpochsPerSyncCommitteePeriod)*uint64(params.BeaconConfig().SlotsPerEpoch) + 1
	pubkeys, err := c.HeadSyncCommitteePubKeys(context.Background(), primitives.Slot(slot), 0)
	require.NoError(t, err)

	// Any subcommittee should match the subcommittee size.
	subCommitteeSize := params.BeaconConfig().SyncCommitteeSize / params.BeaconConfig().SyncCommitteeSubnetCount
	require.Equal(t, int(subCommitteeSize), len(pubkeys))
}

func TestService_HeadSyncCommitteeDomain(t *testing.T) {
	s, _ := util.DeterministicGenesisStateZond(t, params.BeaconConfig().TargetCommitteeSize)
	c := &Service{cfg: &config{BeaconDB: dbTest.SetupDB(t)}}
	c.head = &head{state: s}

	wanted, err := signing.Domain(s.Fork(), slots.ToEpoch(s.Slot()), params.BeaconConfig().DomainSyncCommittee, s.GenesisValidatorsRoot())
	require.NoError(t, err)

	d, err := c.HeadSyncCommitteeDomain(context.Background(), 0)
	require.NoError(t, err)

	require.DeepEqual(t, wanted, d)
}

func TestService_HeadSyncContributionProofDomain(t *testing.T) {
	s, _ := util.DeterministicGenesisStateZond(t, params.BeaconConfig().TargetCommitteeSize)
	c := &Service{cfg: &config{BeaconDB: dbTest.SetupDB(t)}}
	c.head = &head{state: s}

	wanted, err := signing.Domain(s.Fork(), slots.ToEpoch(s.Slot()), params.BeaconConfig().DomainContributionAndProof, s.GenesisValidatorsRoot())
	require.NoError(t, err)

	d, err := c.HeadSyncContributionProofDomain(context.Background(), 0)
	require.NoError(t, err)

	require.DeepEqual(t, wanted, d)
}

func TestService_HeadSyncSelectionProofDomain(t *testing.T) {
	s, _ := util.DeterministicGenesisStateZond(t, params.BeaconConfig().TargetCommitteeSize)
	c := &Service{cfg: &config{BeaconDB: dbTest.SetupDB(t)}}
	c.head = &head{state: s}

	wanted, err := signing.Domain(s.Fork(), slots.ToEpoch(s.Slot()), params.BeaconConfig().DomainSyncCommitteeSelectionProof, s.GenesisValidatorsRoot())
	require.NoError(t, err)

	d, err := c.HeadSyncSelectionProofDomain(context.Background(), 0)
	require.NoError(t, err)

	require.DeepEqual(t, wanted, d)
}

func TestSyncCommitteeHeadStateCache_RoundTrip(t *testing.T) {
	c := syncCommitteeHeadStateCache
	t.Cleanup(func() {
		syncCommitteeHeadStateCache = cache.NewSyncCommitteeHeadState()
	})
	beaconState, _ := util.DeterministicGenesisStateZond(t, 100)
	require.NoError(t, beaconState.SetSlot(100))
	root := [32]byte{1}
	cachedState, err := c.Get(root, 101)
	require.ErrorContains(t, cache.ErrNotFound.Error(), err)
	require.Equal(t, nil, cachedState)
	require.NoError(t, c.Put(root, 101, beaconState))
	cachedState, err = c.Get(root, 101)
	require.NoError(t, err)
	require.DeepEqual(t, beaconState, cachedState)
}

// A period with no blocks leaves the same latest header on both sides of the
// boundary. Advancing the head across it must not change the answers for the
// last slot of the old period, which gossip validation still asks about.
func TestService_HeadSyncCommitteeIndices_BlocklessPeriodBoundary(t *testing.T) {
	ctx := context.Background()
	cfg := params.BeaconConfig()
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	syncCommitteeHeadStateCache = cache.NewSyncCommitteeHeadState()
	t.Cleanup(func() { syncCommitteeHeadStateCache = cache.NewSyncCommitteeHeadState() })
	transition.SkipSlotCache.Disable()
	t.Cleanup(transition.SkipSlotCache.Enable)

	st, _ := util.DeterministicGenesisStateZond(t, 2*cfg.SyncCommitteeSize)
	st, err := transition.ProcessSlots(ctx, st, 1)
	require.NoError(t, err)
	root, err := st.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)
	c := &Service{head: &head{state: st, root: root}}
	boundary := cfg.SlotsPerEpoch * primitives.Slot(cfg.EpochsPerSyncCommitteePeriod)
	lastSlot := boundary - 1

	// Membership for the last slot of a period comes from the next committee.
	committee, err := st.NextSyncCommittee()
	require.NoError(t, err)
	wanted := make([][]primitives.CommitteeIndex, st.NumValidators())
	for i := range wanted {
		key := st.PubkeyAtIndex(primitives.ValidatorIndex(i))
		for position, pubkey := range committee.Pubkeys {
			if bytes.Equal(key[:], pubkey) {
				wanted[i] = append(wanted[i], primitives.CommitteeIndex(position))
			}
		}
		got, err := c.HeadSyncCommitteeIndices(ctx, primitives.ValidatorIndex(i), lastSlot)
		require.NoError(t, err)
		require.Equal(t, true, samePositions(wanted[i], got), "cold lookup for validator %d", i)
	}

	// A request for the boundary slot advances the head across the period
	// boundary, which rotates the committees and writes the position cache.
	_, err = c.HeadSyncCommitteeDomain(ctx, boundary)
	require.NoError(t, err)
	advanced, err := syncCommitteeHeadStateCache.Get(root, boundary)
	require.NoError(t, err)
	rotatedNext, err := advanced.NextSyncCommittee()
	require.NoError(t, err)
	require.DeepNotEqual(t, committee.Pubkeys, rotatedNext.Pubkeys, "rotation must install a different next committee")

	for i := range wanted {
		got, err := c.HeadSyncCommitteeIndices(ctx, primitives.ValidatorIndex(i), lastSlot)
		require.NoError(t, err)
		require.Equal(t, true, samePositions(wanted[i], got), "lookup for validator %d after the head advanced", i)
	}
}

func samePositions(a, b []primitives.CommitteeIndex) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
