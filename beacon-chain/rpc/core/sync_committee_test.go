package core

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	"github.com/theQRL/qrysm/beacon-chain/operations/synccommittee"
	p2pmock "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/beacon-chain/rpc/testutil"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestSubmitSyncMessage_Validation(t *testing.T) {
	for _, tt := range []struct {
		name string
		edit func(*testutil.SyncCommitteeFixture, *qrysmpb.SyncCommitteeMessage) *qrysmpb.SyncCommitteeMessage
		want string
	}{
		{name: "valid"},
		{name: "nil", edit: func(_ *testutil.SyncCommitteeFixture, _ *qrysmpb.SyncCommitteeMessage) *qrysmpb.SyncCommitteeMessage {
			return nil
		}, want: "can't be nil"},
		{name: "forged signature", edit: func(_ *testutil.SyncCommitteeFixture, m *qrysmpb.SyncCommitteeMessage) *qrysmpb.SyncCommitteeMessage {
			m.Signature[0] ^= 1
			return m
		}, want: "invalid sync committee signature"},
		{name: "wrong root", edit: func(_ *testutil.SyncCommitteeFixture, m *qrysmpb.SyncCommitteeMessage) *qrysmpb.SyncCommitteeMessage {
			m.BlockRoot[0] ^= 1
			return m
		}, want: "invalid sync committee signature"},
		{name: "short signature", edit: func(_ *testutil.SyncCommitteeFixture, m *qrysmpb.SyncCommitteeMessage) *qrysmpb.SyncCommitteeMessage {
			m.Signature = m.Signature[:10]
			return m
		}, want: "length"},
		{name: "long root", edit: func(_ *testutil.SyncCommitteeFixture, m *qrysmpb.SyncCommitteeMessage) *qrysmpb.SyncCommitteeMessage {
			m.BlockRoot = append(m.BlockRoot, 0)
			return m
		}, want: "length"},
		{name: "nonmember", edit: func(f *testutil.SyncCommitteeFixture, m *qrysmpb.SyncCommitteeMessage) *qrysmpb.SyncCommitteeMessage {
			f.Head.SyncCommitteeIndices = nil
			return m
		}, want: "not in the sync committee"},
		{name: "wrong domain", edit: func(f *testutil.SyncCommitteeFixture, m *qrysmpb.SyncCommitteeMessage) *qrysmpb.SyncCommitteeMessage {
			f.Head.SyncCommitteeDomain[0] ^= 1
			return m
		}, want: "invalid sync committee signature"},
		{name: "far future", edit: func(_ *testutil.SyncCommitteeFixture, m *qrysmpb.SyncCommitteeMessage) *qrysmpb.SyncCommitteeMessage {
			m.Slot = ^primitives.Slot(0)
			return m
		}, want: "exceeds max allowed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := testutil.NewSyncCommitteeFixture(t, nil, 1)
			broadcaster := &p2pmock.MockBroadcaster{}
			s := &Service{HeadFetcher: f.Head, GenesisTimeFetcher: f.Head, P2P: broadcaster, SyncCommitteePool: synccommittee.NewStore()}
			msg := f.Message(t, 0, make([]byte, 32))
			if tt.edit != nil {
				msg = tt.edit(f, msg)
			}
			if tt.name == "far future" {
				s.HeadFetcher = nil // Must reject before any expensive head-state lookup.
			}
			err := s.SubmitSyncMessage(context.Background(), msg)
			saved, poolErr := s.SyncCommitteePool.SyncCommitteeMessages(1)
			require.NoError(t, poolErr)
			if tt.want == "" {
				require.Equal(t, (*RpcError)(nil), err)
				assert.DeepEqual(t, []*qrysmpb.SyncCommitteeMessage{msg}, saved)
				assert.Equal(t, true, broadcaster.BroadcastCalled)
			} else {
				require.NotNil(t, err)
				assert.Equal(t, ErrorReason(BadRequest), err.Reason)
				assert.ErrorContains(t, tt.want, err.Err)
				assert.Equal(t, 0, len(saved))
				assert.Equal(t, false, broadcaster.BroadcastCalled)
			}
		})
	}
}

func TestSubmitSyncMessage_InvalidCannotOverwrite(t *testing.T) {
	f := testutil.NewSyncCommitteeFixture(t, nil, 1)
	broadcaster := &p2pmock.MockBroadcaster{}
	s := &Service{HeadFetcher: f.Head, GenesisTimeFetcher: f.Head, P2P: broadcaster, SyncCommitteePool: synccommittee.NewStore()}
	msg := f.Message(t, 0, make([]byte, 32))
	require.Equal(t, (*RpcError)(nil), s.SubmitSyncMessage(context.Background(), msg))
	forged := qrysmpb.CopySyncCommitteeMessage(msg)
	forged.BlockRoot[0] ^= 1
	broadcaster.BroadcastCalled = false
	require.NotNil(t, s.SubmitSyncMessage(context.Background(), forged))
	saved, err := s.SyncCommitteePool.SyncCommitteeMessages(1)
	require.NoError(t, err)
	assert.DeepEqual(t, []*qrysmpb.SyncCommitteeMessage{msg}, saved)
	assert.Equal(t, false, broadcaster.BroadcastCalled)
}

func TestSubmitSignedContributionAndProof_Validation(t *testing.T) {
	for _, tt := range []struct {
		name string
		edit func(*testing.T, *testutil.SyncCommitteeFixture, *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof
		want string
	}{
		{name: "valid"},
		{name: "nil", edit: func(_ *testing.T, _ *testutil.SyncCommitteeFixture, _ *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			return nil
		}, want: "can't be nil"},
		{name: "nil message", edit: func(_ *testing.T, _ *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			r.Message = nil
			return r
		}, want: "can't be nil"},
		{name: "nil contribution", edit: func(_ *testing.T, _ *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			r.Message.Contribution = nil
			return r
		}, want: "can't be nil"},
		{name: "forged outer signature", edit: func(_ *testing.T, _ *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			r.Signature[0] ^= 1
			return r
		}, want: "invalid sync contribution signature"},
		{name: "forged selection proof", edit: func(t *testing.T, f *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			r.Message.SelectionProof[0] ^= 1
			r.Signature = f.Sign(t, r.Message, f.Head.SyncContributionProofDomain)
			return r
		}, want: "invalid sync selection proof"},
		{name: "forged participant signed by aggregator", edit: func(t *testing.T, f *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			r.Message.Contribution.Signatures[0][0] ^= 1
			r.Signature = f.Sign(t, r.Message, f.Head.SyncContributionProofDomain)
			return r
		}, want: "invalid sync contribution participant signatures"},
		{name: "wrong participant domain", edit: func(_ *testing.T, f *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			f.Head.SyncCommitteeDomain[0] ^= 1
			return r
		}, want: "invalid sync contribution participant signatures"},
		{name: "nonmember aggregator", edit: func(_ *testing.T, f *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			f.Head.SyncCommitteeIndices = nil
			return r
		}, want: "not in the sync subcommittee"},
		{name: "unselected aggregator", edit: func(t *testing.T, f *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.TargetAggregatorsPerSyncSubcommittee = 1
			params.OverrideBeaconConfig(cfg)
			for i := primitives.ValidatorIndex(0); ; i++ {
				selected, err := altair.IsSyncCommitteeAggregator(f.Head.AggregatorSelectionSeed[:], f.Slot, 0, i)
				require.NoError(t, err)
				if !selected {
					r.Message.AggregatorIndex = i
					break
				}
				require.Equal(t, true, i < 1024)
			}
			r.Signature = f.Sign(t, r.Message, f.Head.SyncContributionProofDomain)
			return r
		}, want: "not a sync committee aggregator"},
		{name: "invalid subnet", edit: func(_ *testing.T, _ *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			r.Message.Contribution.SubcommitteeIndex = params.BeaconConfig().SyncCommitteeSubnetCount
			return r
		}, want: "invalid sync subcommittee index"},
		{name: "long bitvector", edit: func(_ *testing.T, _ *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			r.Message.Contribution.AggregationBits = append(r.Message.Contribution.AggregationBits, 0)
			return r
		}, want: "aggregation bits length"},
		{name: "empty bitvector", edit: func(_ *testing.T, _ *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			r.Message.Contribution.AggregationBits.SetBitAt(0, false)
			r.Message.Contribution.Signatures = nil
			return r
		}, want: "nonempty aggregation bits"},
		{name: "missing signature", edit: func(_ *testing.T, _ *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			r.Message.Contribution.Signatures = nil
			return r
		}, want: "nonempty aggregation bits"},
		{name: "extra signature", edit: func(_ *testing.T, _ *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			c := r.Message.Contribution
			c.Signatures = append(c.Signatures, c.Signatures[0])
			return r
		}, want: "nonempty aggregation bits"},
		{name: "short signature", edit: func(_ *testing.T, _ *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			r.Message.Contribution.Signatures[0] = []byte{1}
			return r
		}, want: "participant signature length"},
		{name: "far future", edit: func(_ *testing.T, _ *testutil.SyncCommitteeFixture, r *qrysmpb.SignedContributionAndProof) *qrysmpb.SignedContributionAndProof {
			r.Message.Contribution.Slot = ^primitives.Slot(0)
			return r
		}, want: "exceeds max allowed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := testutil.NewSyncCommitteeFixture(t, nil, 1)
			broadcaster := &p2pmock.MockBroadcaster{}
			s := &Service{HeadFetcher: f.Head, GenesisTimeFetcher: f.Head, Broadcaster: broadcaster, SyncCommitteePool: synccommittee.NewStore(), OperationNotifier: f.Head.OperationNotifier()}
			events := make(chan *feed.Event, 1)
			sub := s.OperationNotifier.OperationFeed().Subscribe(events)
			defer sub.Unsubscribe()
			req := f.Contribution(t)
			if tt.edit != nil {
				req = tt.edit(t, f, req)
			}
			if tt.name == "far future" {
				s.HeadFetcher = nil
			}
			err := s.SubmitSignedContributionAndProof(context.Background(), req)
			saved, poolErr := s.SyncCommitteePool.SyncCommitteeContributions(1)
			require.NoError(t, poolErr)
			if tt.want == "" {
				require.Equal(t, (*RpcError)(nil), err)
				assert.DeepEqual(t, []*qrysmpb.SyncCommitteeContribution{req.Message.Contribution}, saved)
				assert.Equal(t, true, broadcaster.BroadcastCalled)
				assert.Equal(t, 1, len(events))
			} else {
				require.NotNil(t, err)
				assert.Equal(t, ErrorReason(BadRequest), err.Reason)
				assert.ErrorContains(t, tt.want, err.Err)
				assert.Equal(t, 0, len(saved))
				assert.Equal(t, false, broadcaster.BroadcastCalled)
				assert.Equal(t, 0, len(events))
			}
		})
	}
}

func TestSubmitSignedContributionAndProof_ParticipantOrder(t *testing.T) {
	f := testutil.NewSyncCommitteeFixture(t, nil, 1)
	other := testutil.NewSyncCommitteeFixture(t, nil, 1)
	f.Head.SyncCommitteePubkeys[3] = other.Key.PublicKey().Marshal()
	req := f.Contribution(t)
	c := req.Message.Contribution
	c.AggregationBits.SetBitAt(3, true)
	c.Signatures = append(c.Signatures, other.Message(t, 1, c.BlockRoot).Signature)
	req.Signature = f.Sign(t, req.Message, f.Head.SyncContributionProofDomain)
	broadcaster := &p2pmock.MockBroadcaster{}
	s := &Service{HeadFetcher: f.Head, GenesisTimeFetcher: f.Head, Broadcaster: broadcaster, SyncCommitteePool: synccommittee.NewStore(), OperationNotifier: f.Head.OperationNotifier()}
	require.Equal(t, (*RpcError)(nil), s.SubmitSignedContributionAndProof(context.Background(), req))

	c.Signatures[0], c.Signatures[1] = c.Signatures[1], c.Signatures[0]
	req.Signature = f.Sign(t, req.Message, f.Head.SyncContributionProofDomain)
	broadcaster.BroadcastCalled = false
	err := s.SubmitSignedContributionAndProof(context.Background(), req)
	require.NotNil(t, err)
	assert.ErrorContains(t, "invalid sync contribution participant signatures", err.Err)
	assert.Equal(t, false, broadcaster.BroadcastCalled)
	saved, poolErr := s.SyncCommitteePool.SyncCommitteeContributions(1)
	require.NoError(t, poolErr)
	assert.Equal(t, 1, len(saved))
}
