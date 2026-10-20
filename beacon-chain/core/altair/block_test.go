package altair_test

import (
	"context"
	"math"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	p2pType "github.com/theQRL/qrysm/beacon-chain/p2p/types"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
)

func TestProcessSyncCommittee_PerfectParticipation(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, testValidatorSetSize)
	require.NoError(t, beaconState.SetSlot(1))
	committee, err := altair.NextSyncCommittee(context.Background(), beaconState)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetCurrentSyncCommittee(committee))

	syncBits := bitfield.NewBitvector128()
	for i := range syncBits {
		syncBits[i] = 0xff
	}
	indices, err := altair.NextSyncCommitteeIndices(context.Background(), beaconState)
	require.NoError(t, err)
	ps := slots.PrevSlot(beaconState.Slot())
	pbr, err := helpers.BlockRootAtSlot(beaconState, ps)
	require.NoError(t, err)
	sigs := make([][]byte, len(indices))
	for i, indice := range indices {
		b := p2pType.SSZBytes(pbr)
		sb, err := signing.ComputeDomainAndSign(beaconState, time.CurrentEpoch(beaconState), &b, params.BeaconConfig().DomainSyncCommittee, privKeys[indice])
		require.NoError(t, err)
		sigs[i] = sb
	}
	syncAggregate := &qrysmpb.SyncAggregate{
		SyncCommitteeBits:       syncBits,
		SyncCommitteeSignatures: sigs,
	}

	var reward uint64
	beaconState, reward, err = altair.ProcessSyncAggregate(context.Background(), beaconState, syncAggregate)
	require.NoError(t, err)
	assert.Equal(t, uint64(28908544), reward)

	// Use a non-sync committee index to compare profitability.
	syncCommittee := make(map[primitives.ValidatorIndex]bool)
	for _, index := range indices {
		syncCommittee[index] = true
	}
	nonSyncIndex := primitives.ValidatorIndex(testValidatorSetSize + 1)
	for i := primitives.ValidatorIndex(0); uint64(i) < testValidatorSetSize; i++ {
		if !syncCommittee[i] {
			nonSyncIndex = i
			break
		}
	}

	// Sync committee should be more profitable than non sync committee
	balances := beaconState.Balances()
	require.Equal(t, true, balances[indices[0]] > balances[nonSyncIndex])

	// Proposer should be more profitable than rest of the sync committee
	proposerIndex, err := helpers.BeaconProposerIndex(context.Background(), beaconState)
	require.NoError(t, err)
	require.Equal(t, true, balances[proposerIndex] > balances[indices[0]])

	// Sync committee should have the same profits, except you are a proposer
	for i := 1; i < len(indices); i++ {
		if proposerIndex == indices[i-1] || proposerIndex == indices[i] {
			continue
		}
		require.Equal(t, balances[indices[i-1]], balances[indices[i]])
	}

	// Increased balance validator count should equal to sync committee count
	increased := uint64(0)
	for _, balance := range balances {
		if balance > params.BeaconConfig().MaxEffectiveBalance {
			increased++
		}
	}
	expectedIncreased := params.BeaconConfig().SyncCommitteeSize
	if !syncCommittee[proposerIndex] {
		expectedIncreased++
	}
	require.Equal(t, expectedIncreased, increased)
}

func TestProcessSyncCommittee_MixParticipation_BadSignature(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, testValidatorSetSize)
	require.NoError(t, beaconState.SetSlot(1))
	committee, err := altair.NextSyncCommittee(context.Background(), beaconState)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetCurrentSyncCommittee(committee))

	syncBits := bitfield.NewBitvector128()
	for i := range syncBits {
		syncBits[i] = 0xAA
	}
	indices, err := altair.NextSyncCommitteeIndices(context.Background(), beaconState)
	require.NoError(t, err)
	ps := slots.PrevSlot(beaconState.Slot())
	pbr, err := helpers.BlockRootAtSlot(beaconState, ps)
	require.NoError(t, err)
	sigs := make([][]byte, 0)
	for i, indice := range indices {
		if syncBits.BitAt(uint64(i)) {
			b := p2pType.SSZBytes(pbr)
			sb, err := signing.ComputeDomainAndSign(beaconState, time.CurrentEpoch(beaconState), &b, params.BeaconConfig().DomainDeposit /* incorrect domain */, privKeys[indice])
			require.NoError(t, err)
			sigs = append(sigs, sb)
		}
	}
	syncAggregate := &qrysmpb.SyncAggregate{
		SyncCommitteeBits:       syncBits,
		SyncCommitteeSignatures: sigs,
	}

	_, _, err = altair.ProcessSyncAggregate(context.Background(), beaconState, syncAggregate)
	require.ErrorContains(t, "invalid sync committee signature", err)
}

func TestProcessSyncCommittee_MixParticipation_GoodSignature(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, testValidatorSetSize)
	require.NoError(t, beaconState.SetSlot(1))
	committee, err := altair.NextSyncCommittee(context.Background(), beaconState)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetCurrentSyncCommittee(committee))

	syncBits := bitfield.NewBitvector128()
	for i := range syncBits {
		syncBits[i] = 0xAA
	}
	indices, err := altair.NextSyncCommitteeIndices(context.Background(), beaconState)
	require.NoError(t, err)
	ps := slots.PrevSlot(beaconState.Slot())
	pbr, err := helpers.BlockRootAtSlot(beaconState, ps)
	require.NoError(t, err)
	sigs := make([][]byte, 0, len(indices))
	for i, indice := range indices {
		if syncBits.BitAt(uint64(i)) {
			b := p2pType.SSZBytes(pbr)
			sb, err := signing.ComputeDomainAndSign(beaconState, time.CurrentEpoch(beaconState), &b, params.BeaconConfig().DomainSyncCommittee, privKeys[indice])
			require.NoError(t, err)
			sigs = append(sigs, sb)
		}
	}
	syncAggregate := &qrysmpb.SyncAggregate{
		SyncCommitteeBits:       syncBits,
		SyncCommitteeSignatures: sigs,
	}

	_, _, err = altair.ProcessSyncAggregate(context.Background(), beaconState, syncAggregate)
	require.NoError(t, err)
}

// This is a regression test #11696
func TestProcessSyncCommittee_DontPrecompute(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateZond(t, testValidatorSetSize)
	require.NoError(t, beaconState.SetSlot(1))
	committee, err := altair.NextSyncCommittee(context.Background(), beaconState)
	require.NoError(t, err)
	committeeKeys := committee.Pubkeys
	committeeKeys[1] = committeeKeys[0]
	require.NoError(t, beaconState.SetCurrentSyncCommittee(committee))
	idx, ok := beaconState.ValidatorIndexByPubkey(bytesutil.ToBytes2592(committeeKeys[0]))
	require.Equal(t, true, ok)

	syncBits := bitfield.NewBitvector128()
	for i := range syncBits {
		syncBits[i] = 0xFF
	}
	syncBits.SetBitAt(0, false)
	syncAggregate := &qrysmpb.SyncAggregate{
		SyncCommitteeBits: syncBits,
	}
	require.NoError(t, beaconState.UpdateBalancesAtIndex(idx, 0))
	st, votedKeys, _, err := altair.ProcessSyncAggregateEported(context.Background(), beaconState, syncAggregate)
	require.NoError(t, err)
	require.Equal(t, 127, len(votedKeys))
	require.DeepEqual(t, committeeKeys[0], votedKeys[0].Marshal())
	balances := st.Balances()
	require.Equal(t, uint64(1580937), balances[idx])
}

func TestProcessSyncCommittee_ProposerRewardOrder(t *testing.T) {
	ctx := context.Background()
	cfg := params.BeaconConfig()
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	base, keys := util.DeterministicGenesisStateZond(t, 16)
	// At this slot, the real proposer sampler selects validator 0 even with
	// zero effective balance, because its sampled random byte is zero.
	require.NoError(t, base.SetSlot(809))
	validator, err := base.ValidatorAtIndex(0)
	require.NoError(t, err)
	validator.EffectiveBalance = 0
	validator.ExitEpoch = slots.ToEpoch(base.Slot()) + 10
	require.NoError(t, base.UpdateValidatorAtIndex(0, validator))
	proposer, err := helpers.BeaconProposerIndex(ctx, base)
	require.NoError(t, err)
	require.Equal(t, primitives.ValidatorIndex(0), proposer)

	activeBalance, err := helpers.TotalActiveBalance(base)
	require.NoError(t, err)
	proposerReward, participantReward, err := altair.SyncRewards(activeBalance)
	require.NoError(t, err)
	require.Equal(t, true, proposerReward > 0 && 2*proposerReward < participantReward)
	previousSlot := slots.PrevSlot(base.Slot())
	previousRoot, err := helpers.BlockRootAtSlot(base, previousSlot)
	require.NoError(t, err)
	rootToSign := p2pType.SSZBytes(previousRoot)
	signature, err := signing.ComputeDomainAndSign(base, slots.ToEpoch(previousSlot), &rootToSign, cfg.DomainSyncCommittee, keys[1])
	require.NoError(t, err)
	validators := base.Validators()
	last := cfg.SyncCommitteeSize - 1
	cases := []struct {
		name              string
		initialBalance    uint64
		proposerPositions []uint64
		votePositions     []uint64
		wantBalance       uint64
	}{
		{"reward_then_penalty_empty", 0, []uint64{last}, []uint64{0}, 0},
		{"reward_then_penalty_low", 1, []uint64{last}, []uint64{0}, 0},
		{"reward_then_penalty_at_zero", participantReward - proposerReward, []uint64{last}, []uint64{0}, 0},
		{"reward_then_penalty_above_zero", participantReward - proposerReward + 1, []uint64{last}, []uint64{0}, 1},
		{"reward_then_penalty_sufficient", participantReward, []uint64{last}, []uint64{0}, proposerReward},
		{"penalty_then_reward", 0, []uint64{0}, []uint64{last}, proposerReward},
		{"rewards_straddle_penalty", 0, []uint64{1}, []uint64{0, 2}, proposerReward},
		{"rewards_then_repeated_penalties", 0, []uint64{2, last}, []uint64{0, 1}, 0},
	}
	for _, verifySignatures := range []bool{true, false} {
		name := "verify_signatures"
		process := altair.ProcessSyncAggregate
		if !verifySignatures {
			name = "skip_signatures"
			process = altair.ProcessSyncAggregateNoVerifySig
		}
		t.Run(name, func(t *testing.T) {
			for _, tt := range cases {
				t.Run(tt.name, func(t *testing.T) {
					helpers.ClearCache()
					st := base.Copy()
					require.NoError(t, st.UpdateBalancesAtIndex(proposer, tt.initialBalance))
					committee := &qrysmpb.SyncCommittee{Pubkeys: make([][]byte, cfg.SyncCommitteeSize)}
					for i := range committee.Pubkeys {
						committee.Pubkeys[i] = validators[1].PublicKey
					}
					for _, position := range tt.proposerPositions {
						committee.Pubkeys[position] = validators[proposer].PublicKey
					}
					require.NoError(t, st.SetCurrentSyncCommittee(committee))
					bits := bitfield.NewBitvector128()
					require.Equal(t, cfg.SyncCommitteeSize, bits.Len())
					signatures := make([][]byte, len(tt.votePositions))
					for i, position := range tt.votePositions {
						bits.SetBitAt(position, true)
						signatures[i] = signature
					}
					votes := uint64(len(tt.votePositions))
					misses := cfg.SyncCommitteeSize - uint64(len(tt.proposerPositions)) - votes
					want := st.Balances()
					want[proposer] = tt.wantBalance
					want[1] += votes * participantReward
					want[1] -= misses * participantReward
					post, earned, err := process(ctx, st, &qrysmpb.SyncAggregate{
						SyncCommitteeBits:       bits,
						SyncCommitteeSignatures: signatures,
					})
					require.NoError(t, err)
					// Report all earned rewards even when a subsequent penalty
					// consumes some or all of the proposer's credited balance.
					require.Equal(t, votes*proposerReward, earned)
					require.DeepEqual(t, want, post.Balances())
				})
			}
		})
	}
}

func TestProcessSyncCommittee_processSyncAggregate(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateZond(t, testValidatorSetSize)
	require.NoError(t, beaconState.SetSlot(1))
	committee, err := altair.NextSyncCommittee(context.Background(), beaconState)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetCurrentSyncCommittee(committee))

	syncBits := bitfield.NewBitvector128()
	for i := range syncBits {
		syncBits[i] = 0xAA
	}
	syncAggregate := &qrysmpb.SyncAggregate{
		SyncCommitteeBits: syncBits,
	}

	st, votedKeys, _, err := altair.ProcessSyncAggregateEported(context.Background(), beaconState, syncAggregate)
	require.NoError(t, err)
	votedMap := make(map[[field_params.MLDSA87PubkeyLength]byte]bool)
	for _, key := range votedKeys {
		votedMap[bytesutil.ToBytes2592(key.Marshal())] = true
	}
	require.Equal(t, int(syncBits.Len()/2), len(votedKeys))

	currentSyncCommittee, err := st.CurrentSyncCommittee()
	require.NoError(t, err)
	committeeKeys := currentSyncCommittee.Pubkeys
	balances := st.Balances()

	proposerIndex, err := helpers.BeaconProposerIndex(context.Background(), beaconState)
	require.NoError(t, err)

	for i := range syncBits {
		if syncBits.BitAt(uint64(i)) {
			pk := bytesutil.ToBytes2592(committeeKeys[i])
			require.DeepEqual(t, true, votedMap[pk])
			idx, ok := st.ValidatorIndexByPubkey(pk)
			require.Equal(t, true, ok)
			require.Equal(t, uint64(40000001580937), balances[idx])
		} else {
			pk := bytesutil.ToBytes2592(committeeKeys[i])
			require.DeepEqual(t, false, votedMap[pk])
			idx, ok := st.ValidatorIndexByPubkey(pk)
			require.Equal(t, true, ok)
			if idx != proposerIndex {
				require.Equal(t, uint64(39999998419063), balances[idx])
			}
		}
	}
	require.Equal(t, uint64(40000014454272), balances[proposerIndex])
}

func Test_VerifySyncCommitteeSigs(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, testValidatorSetSize)
	require.NoError(t, beaconState.SetSlot(1))
	committee, err := altair.NextSyncCommittee(context.Background(), beaconState)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetCurrentSyncCommittee(committee))

	syncBits := bitfield.NewBitvector512()
	for i := range syncBits {
		syncBits[i] = 0xff
	}
	indices, err := altair.NextSyncCommitteeIndices(context.Background(), beaconState)
	require.NoError(t, err)
	ps := slots.PrevSlot(beaconState.Slot())
	pbr, err := helpers.BlockRootAtSlot(beaconState, ps)
	require.NoError(t, err)
	sigs := make([][]byte, len(indices))
	sigsBad := make([][]byte, len(indices))
	pks := make([]ml_dsa_87.PublicKey, len(indices))
	for i, indice := range indices {
		b := p2pType.SSZBytes(pbr)
		sb, err := signing.ComputeDomainAndSign(beaconState, time.CurrentEpoch(beaconState), &b, params.BeaconConfig().DomainSyncCommittee, privKeys[indice])
		require.NoError(t, err)
		sigs[i] = sb
		sigsBad[i] = make([]byte, field_params.MLDSA87SignatureLength)
		pks[i] = privKeys[indice].PublicKey()
	}

	mlDSA87Key, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	lsig1, err := mlDSA87Key.Sign([]byte{'m', 'e', 'o', 'w'})
	require.NoError(t, err)
	require.ErrorContains(t, "provided signatures and pubkeys have differing lengths", altair.VerifySyncCommitteeSigs(beaconState, pks, [][]byte{lsig1.Marshal()}))
	require.ErrorContains(t, "invalid sync committee signature", altair.VerifySyncCommitteeSigs(beaconState, pks, sigsBad))
	require.NoError(t, altair.VerifySyncCommitteeSigs(beaconState, pks, sigs))
}

func Test_SyncRewards(t *testing.T) {
	tests := []struct {
		name                  string
		activeBalance         uint64
		wantProposerReward    uint64
		wantParticipantReward uint64
		errString             string
	}{
		{
			name:                  "active balance is 0",
			activeBalance:         0,
			wantProposerReward:    0,
			wantParticipantReward: 0,
			errString:             "active balance can't be 0",
		},
		{
			name:                  "active balance is 1",
			activeBalance:         1,
			wantProposerReward:    0,
			wantParticipantReward: 0,
			errString:             "",
		},
		{
			name:                  "active balance is 1qrl",
			activeBalance:         params.BeaconConfig().EffectiveBalanceIncrement,
			wantProposerReward:    17,
			wantParticipantReward: 123,
			errString:             "",
		},
		{
			name:                  "active balance is 40000qrl",
			activeBalance:         params.BeaconConfig().MaxEffectiveBalance,
			wantProposerReward:    3529,
			wantParticipantReward: 24705,
			errString:             "",
		},
		{
			name:                  "active balance is 40000qrl * 1m validators",
			activeBalance:         params.BeaconConfig().MaxEffectiveBalance * 1e9,
			wantProposerReward:    1522248,
			wantParticipantReward: 10655741,
			errString:             "",
		},
		{
			name:                  "active balance is max uint64",
			activeBalance:         math.MaxUint64,
			wantProposerReward:    2392537,
			wantParticipantReward: 16747761,
			errString:             "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proposerReward, participantReward, err := altair.SyncRewards(tt.activeBalance)
			if (err != nil) && (tt.errString != "") {
				require.ErrorContains(t, tt.errString, err)
				return
			}
			require.Equal(t, tt.wantProposerReward, proposerReward)
			require.Equal(t, tt.wantParticipantReward, participantReward)
		})
	}
}
