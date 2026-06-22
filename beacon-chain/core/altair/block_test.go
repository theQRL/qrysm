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
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, params.BeaconConfig().MaxValidatorsPerCommittee)
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
	assert.Equal(t, uint64(20442752), reward)

	// Use a non-sync committee index to compare profitability.
	syncCommittee := make(map[primitives.ValidatorIndex]bool)
	for _, index := range indices {
		syncCommittee[index] = true
	}
	nonSyncIndex := primitives.ValidatorIndex(params.BeaconConfig().MaxValidatorsPerCommittee + 1)
	for i := primitives.ValidatorIndex(0); uint64(i) < params.BeaconConfig().MaxValidatorsPerCommittee; i++ {
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
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, params.BeaconConfig().MaxValidatorsPerCommittee)
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
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, params.BeaconConfig().MaxValidatorsPerCommittee)
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
	beaconState, _ := util.DeterministicGenesisStateZond(t, params.BeaconConfig().MaxValidatorsPerCommittee)
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
	require.Equal(t, uint64(1117968), balances[idx])
}

func TestProcessSyncCommittee_processSyncAggregate(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateZond(t, params.BeaconConfig().MaxValidatorsPerCommittee)
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
			require.Equal(t, uint64(40000001117968), balances[idx])
		} else {
			pk := bytesutil.ToBytes2592(committeeKeys[i])
			require.DeepEqual(t, false, votedMap[pk])
			idx, ok := st.ValidatorIndexByPubkey(pk)
			require.Equal(t, true, ok)
			if idx != proposerIndex {
				require.Equal(t, uint64(39999998882032), balances[idx])
			}
		}
	}
	require.Equal(t, uint64(40000010221376), balances[proposerIndex])
}

func Test_VerifySyncCommitteeSigs(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, params.BeaconConfig().MaxValidatorsPerCommittee)
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
	require.ErrorContains(t, "provided signatures and pubkeys have differing lengths", altair.VerifySyncCommitteeSigs(beaconState, pks, [][]byte{mlDSA87Key.Sign([]byte{'m', 'e', 'o', 'w'}).Marshal()}))
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
