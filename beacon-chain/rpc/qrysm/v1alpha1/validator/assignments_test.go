package validator

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	mockChain "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/cache/depositcache"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/core/validators"
	mockExecution "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/beacon-chain/rpc/core"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen/mock"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pubKey is a helper to generate a well-formed public key.
func pubKey(i uint64) []byte {
	pubKey := make([]byte, field_params.MLDSA87PubkeyLength)
	binary.LittleEndian.PutUint64(pubKey, i)
	return pubKey
}

func TestGetDuties_OK(t *testing.T) {
	genesis := util.NewBeaconBlockZond()
	depChainStart := params.BeaconConfig().MinGenesisActiveValidatorCount
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	executionData, err := util.DeterministicExecutionData(len(deposits))
	require.NoError(t, err)
	bs, err := transition.GenesisBeaconStateZond(context.Background(), deposits, 0, executionData, &enginev1.ExecutionPayloadZond{})
	require.NoError(t, err, "Could not setup genesis bs")
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	pubKeys := make([][]byte, len(deposits))
	indices := make([]uint64, len(deposits))
	for i := range deposits {
		pubKeys[i] = deposits[i].Data.PublicKey
		indices[i] = uint64(i)
	}

	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now(),
	}
	vs := &Server{
		HeadFetcher:            chain,
		TimeFetcher:            chain,
		SyncChecker:            &mockSync.Sync{IsSyncing: false},
		ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
	}

	// Test the first validator in registry.
	req := &qrysmpb.DutiesRequest{
		PublicKeys: [][]byte{deposits[0].Data.PublicKey},
	}
	res, err := vs.GetDuties(context.Background(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	currentSeed, err := helpers.AggregatorSelectionSeed(bs, 0)
	require.NoError(t, err)
	nextSeed, err := helpers.AggregatorSelectionSeed(bs, 1)
	require.NoError(t, err)
	require.DeepEqual(t, currentSeed[:], res.CurrentEpochDuties[0].AggregatorSelectionSeed)
	require.DeepEqual(t, nextSeed[:], res.NextEpochDuties[0].AggregatorSelectionSeed)
	require.NotEqual(t, currentSeed, nextSeed)
	if res.CurrentEpochDuties[0].AttesterSlot > bs.Slot()+params.BeaconConfig().SlotsPerEpoch {
		t.Errorf("Assigned slot %d can't be higher than %d",
			res.CurrentEpochDuties[0].AttesterSlot, bs.Slot()+params.BeaconConfig().SlotsPerEpoch)
	}

	// Test the last validator in registry.
	lastValidatorIndex := depChainStart - 1
	req = &qrysmpb.DutiesRequest{
		PublicKeys: [][]byte{deposits[lastValidatorIndex].Data.PublicKey},
	}
	res, err = vs.GetDuties(context.Background(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	if res.CurrentEpochDuties[0].AttesterSlot > bs.Slot()+params.BeaconConfig().SlotsPerEpoch {
		t.Errorf("Assigned slot %d can't be higher than %d",
			res.CurrentEpochDuties[0].AttesterSlot, bs.Slot()+params.BeaconConfig().SlotsPerEpoch)
	}

	// We request for duties for all validators.
	req = &qrysmpb.DutiesRequest{
		PublicKeys: pubKeys,
		Epoch:      0,
	}
	res, err = vs.GetDuties(context.Background(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		assert.Equal(t, primitives.ValidatorIndex(i), res.CurrentEpochDuties[i].ValidatorIndex)
	}
}

func TestGetDuties_HistoricalProposersAfterSlashing(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetTestConfig()
	if fieldparams.Preset == "minimal" {
		cfg = params.MinimalSpecConfig()
	}
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	transition.SkipSlotCache.Disable()
	t.Cleanup(transition.SkipSlotCache.Enable)
	ctx := context.Background()
	const historicalEpoch primitives.Epoch = 2

	// Generate fresh signatures for this config instead of reusing deposits
	// cached by tests with a different genesis fork version.
	balances := make([]uint64, 128)
	for i := range balances {
		balances[i] = cfg.MaxEffectiveBalance
	}
	deposits, depositTrie, err := util.DepositsWithBalance(balances)
	require.NoError(t, err)
	depositRoot, err := depositTrie.HashTreeRoot()
	require.NoError(t, err)
	executionData := &qrysmpb.ExecutionData{
		BlockHash: depositRoot[:], DepositRoot: depositRoot[:], DepositCount: uint64(len(deposits)),
	}
	st, err := transition.GenesisBeaconStateZond(ctx, deposits, 0, executionData, &enginev1.ExecutionPayloadZond{
		ParentHash: make([]byte, 32), FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
		StateRoot: make([]byte, 32), ReceiptsRoot: make([]byte, 32), LogsBloom: make([]byte, 256),
		PrevRandao: make([]byte, 32), BaseFeePerGas: make([]byte, 32), BlockHash: make([]byte, 32),
	})
	require.NoError(t, err)
	require.Equal(t, len(deposits), st.NumValidators())
	st, err = transition.ProcessSlots(ctx, st, primitives.Slot(historicalEpoch)*cfg.SlotsPerEpoch)
	require.NoError(t, err)
	historical := st.Copy()
	want, err := helpers.ProposerAssignments(ctx, historical, historicalEpoch)
	require.NoError(t, err)

	// Real slashing and epoch processing change the effective balances used to
	// sample proposers. Historical duties must still use the earlier balances.
	st, err = transition.ProcessSlots(ctx, st, st.Slot()+1)
	require.NoError(t, err)
	for index := primitives.ValidatorIndex(0); index < 64; index++ {
		st, err = validators.SlashValidator(ctx, st, index, cfg.MinSlashingPenaltyQuotient, cfg.ProposerRewardQuotient)
		require.NoError(t, err)
	}
	st, err = transition.ProcessSlots(ctx, st, primitives.Slot(historicalEpoch+1)*cfg.SlotsPerEpoch)
	require.NoError(t, err)
	before, err := historical.ValidatorAtIndex(0)
	require.NoError(t, err)
	after, err := st.ValidatorAtIndex(0)
	require.NoError(t, err)
	require.Equal(t, true, after.EffectiveBalance < before.EffectiveBalance)
	headProposers, err := helpers.ProposerAssignments(ctx, st, historicalEpoch+1)
	require.NoError(t, err)

	pubkeys := make([][]byte, len(deposits))
	for i, deposit := range deposits {
		pubkeys[i] = deposit.Data.PublicKey
	}
	headSlot := st.Slot()
	chain := &mockChain.ChainService{State: st, Slot: &headSlot}
	for _, name := range []string{"empty_cache", "populated_cache"} {
		t.Run(name, func(t *testing.T) {
			helpers.ClearCache()
			if name == "populated_cache" {
				require.NoError(t, helpers.UpdateProposerIndicesInCache(ctx, historical, historicalEpoch))
			}
			vs := &Server{
				HeadFetcher:            chain,
				TimeFetcher:            chain,
				SyncChecker:            &mockSync.Sync{},
				ReplayerBuilder:        mock.NewMockReplayerBuilder(mock.WithMockState(historical)),
				ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
			}
			for index, slots := range headProposers {
				for _, slot := range slots {
					vs.ProposerSlotIndexCache.SetProposerAndPayloadIDs(slot, index, [8]byte{}, [32]byte{})
				}
			}
			res, err := vs.GetDuties(ctx, &qrysmpb.DutiesRequest{Epoch: historicalEpoch, PublicKeys: pubkeys})
			require.NoError(t, err)
			require.Equal(t, len(pubkeys), len(res.CurrentEpochDuties))
			for _, duty := range res.CurrentEpochDuties {
				assert.DeepEqual(t, want[duty.ValidatorIndex], duty.ProposerSlots, "validator %d", duty.ValidatorIndex)
			}
			// Predictions made from the historical state must not overwrite the
			// current epoch's payload preparation assignments.
			for index, slots := range headProposers {
				for _, slot := range slots {
					got, _, exists := vs.ProposerSlotIndexCache.GetProposerPayloadIDs(slot, [32]byte{})
					require.Equal(t, true, exists)
					assert.Equal(t, index, got, "live proposer at slot %d", slot)
				}
			}
		})
	}
}

func TestGetDuties_HistoricalReplayError(t *testing.T) {
	st, _ := util.DeterministicGenesisStateZond(t, 1)
	headSlot := 3 * params.BeaconConfig().SlotsPerEpoch
	require.NoError(t, st.SetSlot(headSlot))
	chain := &mockChain.ChainService{State: st, Slot: &headSlot}
	replayer := mock.NewMockReplayerBuilder()
	replayer.SetMockSlotError(0, errors.New("historical state unavailable"))
	vs := &Server{
		HeadFetcher:     chain,
		TimeFetcher:     chain,
		SyncChecker:     &mockSync.Sync{},
		ReplayerBuilder: replayer,
	}
	_, err := vs.GetDuties(context.Background(), &qrysmpb.DutiesRequest{Epoch: 0})
	require.ErrorContains(t, "historical state unavailable", err)
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestGetZondDuties_SyncCommitteeOK(t *testing.T) {
	helpers.ClearCache()
	params.SetupTestConfigCleanup(t)

	genesis := util.NewBeaconBlockZond()
	deposits, _, err := util.DeterministicDepositsAndKeys(params.BeaconConfig().SyncCommitteeSize)
	require.NoError(t, err)
	executionData, err := util.DeterministicExecutionData(len(deposits))
	require.NoError(t, err)
	bs, err := util.GenesisBeaconStateZond(context.Background(), deposits, 0, executionData)
	require.NoError(t, err, "Could not setup genesis bs")
	h := &qrysmpb.BeaconBlockHeader{
		StateRoot:  bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength),
		ParentRoot: bytesutil.PadTo([]byte{'b'}, fieldparams.RootLength),
		BodyRoot:   bytesutil.PadTo([]byte{'c'}, fieldparams.RootLength),
	}
	require.NoError(t, bs.SetLatestBlockHeader(h))
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	syncCommittee, err := altair.NextSyncCommittee(context.Background(), bs)
	require.NoError(t, err)
	require.NoError(t, bs.SetCurrentSyncCommittee(syncCommittee))

	var nextSyncCommitteePubKeys [][]byte
	for i := uint64(0); i < params.BeaconConfig().SyncCommitteeSize; i++ {
		nextSyncCommitteePubKeys = append(nextSyncCommitteePubKeys, bytesutil.PadTo([]byte{}, field_params.MLDSA87PubkeyLength))
	}
	nextSyncCommittee := &qrysmpb.SyncCommittee{Pubkeys: nextSyncCommitteePubKeys}

	require.NoError(t, bs.SetNextSyncCommittee(nextSyncCommittee))
	pubKeys := make([][]byte, len(deposits))
	indices := make([]uint64, len(deposits))
	for i := range deposits {
		pubKeys[i] = deposits[i].Data.PublicKey
		indices[i] = uint64(i)
	}
	genesisState := bs.Copy()
	require.NoError(t, bs.SetSlot(params.BeaconConfig().SlotsPerEpoch*primitives.Slot(params.BeaconConfig().EpochsPerSyncCommitteePeriod)-1))
	require.NoError(t, helpers.UpdateSyncCommitteeCache(bs))

	slot := uint64(params.BeaconConfig().SlotsPerEpoch) * uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod) * params.BeaconConfig().SecondsPerSlot
	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now().Add(time.Duration(-1*int64(slot-1)) * time.Second),
	}
	vs := &Server{
		HeadFetcher:            chain,
		TimeFetcher:            chain,
		ExecutionInfoFetcher:   &mockExecution.Chain{},
		SyncChecker:            &mockSync.Sync{IsSyncing: false},
		ReplayerBuilder:        mock.NewMockReplayerBuilder(mock.WithMockState(genesisState)),
		ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
	}

	// Test the first validator in registry.
	req := &qrysmpb.DutiesRequest{
		PublicKeys: [][]byte{deposits[0].Data.PublicKey},
	}
	res, err := vs.GetDuties(context.Background(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	if res.CurrentEpochDuties[0].AttesterSlot > bs.Slot()+params.BeaconConfig().SlotsPerEpoch {
		t.Errorf("Assigned slot %d can't be higher than %d",
			res.CurrentEpochDuties[0].AttesterSlot, bs.Slot()+params.BeaconConfig().SlotsPerEpoch)
	}

	// Test the last validator in registry.
	lastValidatorIndex := params.BeaconConfig().SyncCommitteeSize - 1
	req = &qrysmpb.DutiesRequest{
		PublicKeys: [][]byte{deposits[lastValidatorIndex].Data.PublicKey},
	}
	res, err = vs.GetDuties(context.Background(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	if res.CurrentEpochDuties[0].AttesterSlot > bs.Slot()+params.BeaconConfig().SlotsPerEpoch {
		t.Errorf("Assigned slot %d can't be higher than %d",
			res.CurrentEpochDuties[0].AttesterSlot, bs.Slot()+params.BeaconConfig().SlotsPerEpoch)
	}

	// We request for duties for all validators.
	req = &qrysmpb.DutiesRequest{
		PublicKeys: pubKeys,
		Epoch:      0,
	}
	res, err = vs.GetDuties(context.Background(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		assert.Equal(t, primitives.ValidatorIndex(i), res.CurrentEpochDuties[i].ValidatorIndex)
	}

	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		assert.Equal(t, true, res.CurrentEpochDuties[i].IsSyncCommittee)
		// Current epoch and next epoch duties should be equal before the sync period epoch boundary.
		assert.Equal(t, res.CurrentEpochDuties[i].IsSyncCommittee, res.NextEpochDuties[i].IsSyncCommittee)
	}

	// Current epoch and next epoch duties should not be equal at the sync period epoch boundary.
	req = &qrysmpb.DutiesRequest{
		PublicKeys: pubKeys,
		Epoch:      params.BeaconConfig().EpochsPerSyncCommitteePeriod - 1,
	}
	res, err = vs.GetDuties(context.Background(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	for i := 0; i < len(res.CurrentEpochDuties); i++ {
		require.NotEqual(t, res.CurrentEpochDuties[i].IsSyncCommittee, res.NextEpochDuties[i].IsSyncCommittee)
	}
}

func TestGetAltairDuties_UnknownPubkey(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	genesis := util.NewBeaconBlockZond()
	deposits, _, err := util.DeterministicDepositsAndKeys(params.BeaconConfig().SyncCommitteeSize)
	require.NoError(t, err)
	executionData, err := util.DeterministicExecutionData(len(deposits))
	require.NoError(t, err)
	bs, err := util.GenesisBeaconStateZond(context.Background(), deposits, 0, executionData)
	require.NoError(t, err)
	h := &qrysmpb.BeaconBlockHeader{
		StateRoot:  bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength),
		ParentRoot: bytesutil.PadTo([]byte{'b'}, fieldparams.RootLength),
		BodyRoot:   bytesutil.PadTo([]byte{'c'}, fieldparams.RootLength),
	}
	require.NoError(t, bs.SetLatestBlockHeader(h))
	require.NoError(t, err, "Could not setup genesis bs")
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	genesisState := bs.Copy()
	require.NoError(t, bs.SetSlot(params.BeaconConfig().SlotsPerEpoch*primitives.Slot(params.BeaconConfig().EpochsPerSyncCommitteePeriod)-1))
	require.NoError(t, helpers.UpdateSyncCommitteeCache(bs))

	slot := uint64(params.BeaconConfig().SlotsPerEpoch) * uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod) * params.BeaconConfig().SecondsPerSlot
	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now().Add(time.Duration(-1*int64(slot-1)) * time.Second),
	}
	depositCache, err := depositcache.New()
	require.NoError(t, err)

	vs := &Server{
		HeadFetcher:            chain,
		TimeFetcher:            chain,
		ExecutionInfoFetcher:   &mockExecution.Chain{},
		SyncChecker:            &mockSync.Sync{IsSyncing: false},
		DepositFetcher:         depositCache,
		ReplayerBuilder:        mock.NewMockReplayerBuilder(mock.WithMockState(genesisState)),
		ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
	}

	unknownPubkey := bytesutil.PadTo([]byte{'u'}, 48)
	req := &qrysmpb.DutiesRequest{
		PublicKeys: [][]byte{unknownPubkey},
	}
	res, err := vs.GetDuties(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, false, res.CurrentEpochDuties[0].IsSyncCommittee)
	assert.Equal(t, false, res.NextEpochDuties[0].IsSyncCommittee)
}

func TestGetDuties_SlotOutOfUpperBound(t *testing.T) {
	chain := &mockChain.ChainService{
		Genesis: time.Now(),
	}
	vs := &Server{
		TimeFetcher: chain,
	}
	req := &qrysmpb.DutiesRequest{
		Epoch: primitives.Epoch(chain.CurrentSlot()/params.BeaconConfig().SlotsPerEpoch + 2),
	}
	_, err := vs.duties(context.Background(), req)
	require.ErrorContains(t, "can not be greater than next epoch", err)
}

func TestGetDuties_CurrentEpoch_ShouldNotFail(t *testing.T) {
	genesis := util.NewBeaconBlockZond()
	depChainStart := params.BeaconConfig().MinGenesisActiveValidatorCount
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	executionData, err := util.DeterministicExecutionData(len(deposits))
	require.NoError(t, err)
	bState, err := transition.GenesisBeaconStateZond(context.Background(), deposits, 0, executionData, &enginev1.ExecutionPayloadZond{})
	require.NoError(t, err, "Could not setup genesis state")
	// Set state to non-epoch start slot.
	require.NoError(t, bState.SetSlot(5))

	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	pubKeys := make([][field_params.MLDSA87PubkeyLength]byte, len(deposits))
	indices := make([]uint64, len(deposits))
	for i := range deposits {
		pubKeys[i] = bytesutil.ToBytes2592(deposits[i].Data.PublicKey)
		indices[i] = uint64(i)
	}

	chain := &mockChain.ChainService{
		State: bState, Root: genesisRoot[:], Genesis: time.Now(),
	}
	vs := &Server{
		HeadFetcher:            chain,
		TimeFetcher:            chain,
		SyncChecker:            &mockSync.Sync{IsSyncing: false},
		ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
	}

	// Test the first validator in registry.
	req := &qrysmpb.DutiesRequest{
		PublicKeys: [][]byte{deposits[0].Data.PublicKey},
	}
	res, err := vs.GetDuties(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 1, len(res.CurrentEpochDuties), "Expected 1 assignment")
}

func TestGetDuties_MultipleKeys_OK(t *testing.T) {
	genesis := util.NewBeaconBlockZond()
	depChainStart := uint64(64)

	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	executionData, err := util.DeterministicExecutionData(len(deposits))
	require.NoError(t, err)
	bs, err := transition.GenesisBeaconStateZond(context.Background(), deposits, 0, executionData, &enginev1.ExecutionPayloadZond{})
	require.NoError(t, err, "Could not setup genesis bs")
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	pubKeys := make([][field_params.MLDSA87PubkeyLength]byte, len(deposits))
	indices := make([]uint64, len(deposits))
	for i := range deposits {
		pubKeys[i] = bytesutil.ToBytes2592(deposits[i].Data.PublicKey)
		indices[i] = uint64(i)
	}

	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now(),
	}
	vs := &Server{
		HeadFetcher:            chain,
		TimeFetcher:            chain,
		SyncChecker:            &mockSync.Sync{IsSyncing: false},
		ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
	}

	pubkey0 := deposits[0].Data.PublicKey
	pubkey1 := deposits[1].Data.PublicKey

	// Test the first validator in registry.
	req := &qrysmpb.DutiesRequest{
		PublicKeys: [][]byte{pubkey0, pubkey1},
	}
	res, err := vs.GetDuties(context.Background(), req)
	require.NoError(t, err, "Could not call epoch committee assignment")
	assert.Equal(t, 2, len(res.CurrentEpochDuties))
	// The attester slots follow the committee shuffling of the genesis state.
	assignments, err := helpers.CommitteeAssignments(context.Background(), bs, 0, []primitives.ValidatorIndex{0, 1})
	require.NoError(t, err)
	assert.Equal(t, assignments[0].AttesterSlot, res.CurrentEpochDuties[0].AttesterSlot)
	assert.Equal(t, assignments[1].AttesterSlot, res.CurrentEpochDuties[1].AttesterSlot)
	assert.NotEqual(t, res.CurrentEpochDuties[0].AttesterSlot, res.CurrentEpochDuties[1].AttesterSlot)
}

func TestGetDuties_SyncNotReady(t *testing.T) {
	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: true},
	}
	_, err := vs.GetDuties(context.Background(), &qrysmpb.DutiesRequest{})
	assert.ErrorContains(t, "Syncing to latest head", err)
}

func TestAssignValidatorToSubnet(t *testing.T) {
	k := pubKey(3)

	core.AssignValidatorToSubnetProto(k, qrysmpb.ValidatorStatus_ACTIVE)
	coms, ok, exp := cache.SubnetIDs.GetPersistentSubnets(k)
	require.Equal(t, true, ok, "No cache entry found for validator")
	assert.Equal(t, params.BeaconConfig().RandomSubnetsPerValidator, uint64(len(coms)))
	epochDuration := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	totalTime := time.Duration(params.BeaconConfig().EpochsPerRandomSubnetSubscription) * epochDuration * time.Second
	receivedTime := time.Until(exp.Round(time.Second))
	if receivedTime < totalTime {
		t.Fatalf("Expiration time of %f was less than expected duration of %f ", receivedTime.Seconds(), totalTime.Seconds())
	}
}

func TestAssignValidatorToSyncSubnet(t *testing.T) {
	k := pubKey(3)
	committee := make([][]byte, 0)

	for i := range 100 {
		committee = append(committee, pubKey(uint64(i)))
	}
	sCommittee := &qrysmpb.SyncCommittee{
		Pubkeys: committee,
	}
	registerSyncSubnet(0, 0, k, sCommittee, qrysmpb.ValidatorStatus_ACTIVE)
	coms, _, ok, exp := cache.SyncSubnetIDs.GetSyncCommitteeSubnets(k, 0)
	require.Equal(t, true, ok, "No cache entry found for validator")
	assert.Equal(t, uint64(1), uint64(len(coms)))
	epochDuration := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	totalTime := time.Duration(params.BeaconConfig().EpochsPerSyncCommitteePeriod) * epochDuration * time.Second
	receivedTime := time.Until(exp.Round(time.Second)).Round(time.Second)
	if receivedTime < totalTime {
		t.Fatalf("Expiration time of %f was less than expected duration of %f ", receivedTime.Seconds(), totalTime.Seconds())
	}
}

func BenchmarkCommitteeAssignment(b *testing.B) {
	genesis := util.NewBeaconBlockZond()
	depChainStart := uint64(8192 * 2)
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(b, err)
	executionData, err := util.DeterministicExecutionData(len(deposits))
	require.NoError(b, err)
	bs, err := transition.GenesisBeaconStateZond(context.Background(), deposits, 0, executionData, &enginev1.ExecutionPayloadZond{})
	require.NoError(b, err, "Could not setup genesis bs")
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(b, err, "Could not get signing root")

	pubKeys := make([][field_params.MLDSA87PubkeyLength]byte, len(deposits))
	indices := make([]uint64, len(deposits))
	for i := range deposits {
		pubKeys[i] = bytesutil.ToBytes2592(deposits[i].Data.PublicKey)
		indices[i] = uint64(i)
	}

	vs := &Server{
		HeadFetcher: &mockChain.ChainService{State: bs, Root: genesisRoot[:]},
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}

	// Create request for all validators in the system.
	pks := make([][]byte, len(deposits))
	for i, deposit := range deposits {
		pks[i] = deposit.Data.PublicKey
	}
	req := &qrysmpb.DutiesRequest{
		PublicKeys: pks,
		Epoch:      0,
	}

	for b.Loop() {
		_, err := vs.GetDuties(context.Background(), req)
		assert.NoError(b, err)
	}
}
