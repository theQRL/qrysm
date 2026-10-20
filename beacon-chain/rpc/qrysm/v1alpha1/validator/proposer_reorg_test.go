package validator

import (
	"context"
	"errors"
	"testing"

	chainmock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	dbtest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	exectesting "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/beacon-chain/state"
	statemock "github.com/theQRL/qrysm/beacon-chain/state/stategen/mock"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	payloadattribute "github.com/theQRL/qrysm/consensus-types/payload-attribute"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

type proposalReorgChain struct {
	*chainmock.ChainService
	parentRoot  [32]byte
	replacement state.BeaconState
}

func (*proposalReorgChain) UpdateHead(context.Context, primitives.Slot) {}
func (c *proposalReorgChain) CachedHeadRoot() [32]byte                  { return c.parentRoot }
func (c *proposalReorgChain) GetProposerHead() [32]byte {
	if c.replacement != nil {
		c.State = c.replacement
	}
	return c.parentRoot
}

func TestServer_ParentStateAfterReorg(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerHistoricalRoot = fieldparams.BlockRootsLength
	cfg.EpochsPerHistoricalVector = fieldparams.RandaoMixesLength
	cfg.EpochsPerSlashingsVector = fieldparams.SlashingsLength
	cfg.SyncCommitteeSize = fieldparams.SyncCommitteeLength
	params.OverrideBeaconConfig(cfg)
	transition.SkipSlotCache.Disable()
	t.Cleanup(transition.SkipSlotCache.Enable)
	ctx := context.Background()
	for _, reorg := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged head", true: "head changes after parent selection"}[reorg], func(t *testing.T) {
			oldState, err := util.NewBeaconStateZond(func(st *qrysmpb.BeaconStateZond) error {
				st.Slot = 1
				st.LatestBlockHeader.Slot = 1
				st.LatestBlockHeader.StateRoot = bytesutil.PadTo([]byte{'o'}, 32)
				return nil
			})
			require.NoError(t, err)
			parentRoot, err := oldState.LatestBlockHeader().HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, true, transition.NextSlotState(parentRoot[:], 2) == nil, "exercise the cache miss")
			chain := &proposalReorgChain{ChainService: &chainmock.ChainService{State: oldState.Copy()}, parentRoot: parentRoot}
			if reorg {
				chain.replacement = oldState.Copy()
				header := chain.replacement.LatestBlockHeader()
				header.StateRoot = bytesutil.PadTo([]byte{'n'}, 32)
				require.NoError(t, chain.replacement.SetLatestBlockHeader(header))
			}
			vs := &Server{
				HeadFetcher: chain, ForkchoiceFetcher: chain, TimeFetcher: chain,
				StateGen: &statemock.MockStateManager{StatesByRoot: map[[32]byte]state.BeaconState{parentRoot: oldState.Copy()}},
			}
			got, root, err := vs.getParentState(ctx, 2)
			require.NoError(t, err)
			require.Equal(t, parentRoot, root)
			want, err := transition.ProcessSlots(ctx, oldState.Copy(), 2)
			require.NoError(t, err)
			wantRoot, err := want.HashTreeRoot(ctx)
			require.NoError(t, err)
			gotRoot, err := got.HashTreeRoot(ctx)
			require.NoError(t, err)
			require.Equal(t, wantRoot, gotRoot, "advance the selected parent's state, even after a reorg")
		})
	}
}

type proposalForkchoiceAcknowledgement struct {
	*chainmock.ChainService
	accepted bool
}

func (c *proposalForkchoiceAcknowledgement) InvalidateForkchoiceUpdate() { c.accepted = false }

type proposalUpdatingEngine struct {
	*exectesting.EngineClient
	beforeReply func()
}

func (e *proposalUpdatingEngine) ForkchoiceUpdated(ctx context.Context, f *enginev1.ForkchoiceState, attr payloadattribute.Attributer) (*enginev1.PayloadIDBytes, []byte, error) {
	e.beforeReply()
	return e.EngineClient.ForkchoiceUpdated(ctx, f, attr)
}

func TestServer_PayloadForkchoiceAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cached bool
		rpcErr error
	}{
		{name: "VALID"},
		{name: "RPC failure", rpcErr: errors.New("response lost after execution update")},
		{name: "canceled RPC", rpcErr: context.Canceled},
		{name: "cached payload", cached: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st, _ := util.DeterministicGenesisStateZond(t, 1)
			chain := &proposalForkchoiceAcknowledgement{ChainService: &chainmock.ChainService{}, accepted: true}
			engine := &proposalUpdatingEngine{
				EngineClient: &exectesting.EngineClient{
					PayloadIDBytes: &enginev1.PayloadIDBytes{1}, ExecutionPayloadZond: emptyPayloadZond(), ErrForkchoiceUpdated: tc.rpcErr,
				},
				beforeReply: func() {
					require.Equal(t, false, tc.cached, "a cached payload must not send FCU")
					require.Equal(t, false, chain.accepted, "forget the old acknowledgement before the proposal FCU")
					// A chain-service FCU can acknowledge a new head while this
					// proposal is in flight, before execution accepts its parent.
					chain.accepted = true
				},
			}
			vs := &Server{
				ExecutionEngineCaller: engine, ForkchoiceFetcher: chain, FinalizationFetcher: chain,
				BeaconDB: dbtest.SetupDB(t), ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
			}
			blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
			require.NoError(t, err)
			if tc.cached {
				vs.ProposerSlotIndexCache.SetProposerAndPayloadIDs(blk.Block().Slot(), blk.Block().ProposerIndex(), [8]byte{1}, blk.Block().ParentRoot())
			}
			_, _, err = vs.getLocalPayload(ctx, blk.Block(), st)
			if tc.rpcErr != nil {
				require.ErrorIs(t, err, tc.rpcErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.cached, chain.accepted, "an FCU response, including an error, must leave reconciliation pending")
		})
	}
}
