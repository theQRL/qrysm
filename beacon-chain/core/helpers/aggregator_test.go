package helpers_test

import (
	"bytes"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestAggregatorSelection_Golden(t *testing.T) {
	// Independently calculated SHA-256 vectors for the specified fixed-width
	// little-endian encoding. They detect changes that would split client and BN.
	seed := bytes.Repeat([]byte{0x42}, 32)
	var selected []primitives.ValidatorIndex
	for i := primitives.ValidatorIndex(0); i < 128; i++ {
		ok, err := helpers.IsAggregatorSelected(seed, [4]byte{5, 0, 0, 0}, 128, 3, i, 128, 8)
		require.NoError(t, err)
		if ok {
			selected = append(selected, i)
		}
	}
	require.DeepEqual(t, []primitives.ValidatorIndex{22, 41, 45, 60, 70, 71, 78, 98, 105, 117, 120}, selected)
}

func TestAggregatorSelection_DutySeparation(t *testing.T) {
	seed := bytes.Repeat([]byte{0x42}, 32)
	selection := func(seed []byte, role [4]byte, slot primitives.Slot, committee uint64) []bool {
		result := make([]bool, 256)
		for i := range result {
			var err error
			result[i], err = helpers.IsAggregatorSelected(seed, role, slot, committee, primitives.ValidatorIndex(i), 128, 8)
			require.NoError(t, err)
		}
		return result
	}
	base := selection(seed, [4]byte{5}, 128, 3)
	for name, other := range map[string][]bool{
		"seed":      selection(bytes.Repeat([]byte{0x43}, 32), [4]byte{5}, 128, 3),
		"role":      selection(seed, [4]byte{8}, 128, 3),
		"slot":      selection(seed, [4]byte{5}, 129, 3),
		"committee": selection(seed, [4]byte{5}, 128, 4),
	} {
		t.Run(name, func(t *testing.T) {
			for i := range base {
				if base[i] != other[i] {
					return
				}
			}
			t.Fatal("different duty inputs produced identical selections")
		})
	}
}

func TestAggregatorSelection_RejectsInvalidInputs(t *testing.T) {
	for _, seed := range [][]byte{nil, make([]byte, 31), make([]byte, 33)} {
		ok, err := helpers.IsAggregator(128, seed, 0, 0, 0)
		require.ErrorContains(t, "seed must be 32 bytes", err)
		require.Equal(t, false, ok)
	}
	for _, sizes := range [][2]uint64{{0, 8}, {128, 0}} {
		ok, err := helpers.IsAggregatorSelected(make([]byte, 32), [4]byte{5}, 0, 0, 0, sizes[0], sizes[1])
		require.ErrorContains(t, "must be nonzero", err)
		require.Equal(t, false, ok)
	}
	ok, err := altair.IsSyncCommitteeAggregator(make([]byte, 32), 0, params.BeaconConfig().SyncCommitteeSubnetCount, 0)
	require.ErrorContains(t, "invalid sync subcommittee", err)
	require.Equal(t, false, ok)
}

func TestAggregatorSelectionSeed_ChainAndLookahead(t *testing.T) {
	cfg := params.BeaconConfig()
	epoch := primitives.Epoch(10)
	mixes := make([][]byte, cfg.EpochsPerHistoricalVector)
	for i := range mixes {
		mixes[i] = make([]byte, 32)
	}
	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot:                  cfg.SlotsPerEpoch.Mul(uint64(epoch)),
		GenesisValidatorsRoot: bytes.Repeat([]byte{1}, 32),
		RandaoMixes:           mixes,
	})
	require.NoError(t, err)
	seed, err := helpers.AggregatorSelectionSeed(st, epoch)
	require.NoError(t, err)
	// The current epoch's changing randomness must not affect its lottery.
	require.NoError(t, st.UpdateRandaoMixesAtIndex(uint64(epoch), [32]byte{2}))
	sameSeed, err := helpers.AggregatorSelectionSeed(st, epoch)
	require.NoError(t, err)
	require.Equal(t, seed, sameSeed)
	// Changing the historical mix (a reorg) must refresh eligibility with duties.
	lookahead := (epoch + cfg.EpochsPerHistoricalVector - cfg.MinSeedLookahead - 1) % cfg.EpochsPerHistoricalVector
	require.NoError(t, st.UpdateRandaoMixesAtIndex(uint64(lookahead), [32]byte{3}))
	newSeed, err := helpers.AggregatorSelectionSeed(st, epoch)
	require.NoError(t, err)
	require.NotEqual(t, seed, newSeed)
	require.NoError(t, st.SetGenesisValidatorsRoot(bytes.Repeat([]byte{4}, 32)))
	otherChain, err := helpers.AggregatorSelectionSeed(st, epoch)
	require.NoError(t, err)
	require.NotEqual(t, newSeed, otherChain)
}
