package validator

import (
	"context"
	"testing"

	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/operations/synccommittee"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestServer_SetSyncAggregate_EmptyCase(t *testing.T) {
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
	require.NoError(t, err)
	s := &Server{} // Sever is not initialized with sync committee pool.
	s.setSyncAggregate(context.Background(), b, nil)
	agg, err := b.Block().Body().SyncAggregate()
	require.NoError(t, err)

	want := &qrysmpb.SyncAggregate{
		SyncCommitteeBits:       make([]byte, params.BeaconConfig().SyncCommitteeSize/8),
		SyncCommitteeSignatures: [][]byte{},
	}
	require.DeepEqual(t, want, agg)
}

func TestProposer_GetSyncAggregate_IncludesSyncCommitteeMessages(t *testing.T) {
	helpers.ClearCache()
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	priv0, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	priv1, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	priv2, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	priv3, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	pub0 := priv0.PublicKey().Marshal()
	pub1 := priv1.PublicKey().Marshal()
	pub2 := priv2.PublicKey().Marshal()
	pub3 := priv3.PublicKey().Marshal()

	vals := []*qrysmpb.Validator{
		{PublicKey: pub0},
		{PublicKey: pub1},
		{PublicKey: pub2},
		{PublicKey: pub3},
	}
	require.NoError(t, st.SetValidators(vals))

	sc := &qrysmpb.SyncCommittee{
		Pubkeys: make([][]byte, params.BeaconConfig().SyncCommitteeSize),
	}
	for i := range sc.Pubkeys {
		sc.Pubkeys[i] = make([]byte, len(pub0))
	}
	sc.Pubkeys[0] = pub0
	sc.Pubkeys[1] = pub0
	sc.Pubkeys[2] = pub1
	sc.Pubkeys[3] = pub2
	require.NoError(t, st.SetCurrentSyncCommittee(sc))

	proposerServer := &Server{
		HeadFetcher:       &mock.ChainService{State: st},
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		SyncCommitteePool: synccommittee.NewStore(),
	}

	r := params.BeaconConfig().ZeroHash
	lsig1, err := priv0.Sign([]byte("m0"))
	require.NoError(t, err)
	msg0 := &qrysmpb.SyncCommitteeMessage{Slot: 1, BlockRoot: r[:], ValidatorIndex: 0, Signature: lsig1.Marshal()}
	lsig2, err := priv1.Sign([]byte("m1"))
	require.NoError(t, err)
	msg1 := &qrysmpb.SyncCommitteeMessage{Slot: 1, BlockRoot: r[:], ValidatorIndex: 1, Signature: lsig2.Marshal()}
	lsig3, err := priv2.Sign([]byte("m2"))
	require.NoError(t, err)
	msg2 := &qrysmpb.SyncCommitteeMessage{Slot: 1, BlockRoot: r[:], ValidatorIndex: 2, Signature: lsig3.Marshal()}
	msgs := []*qrysmpb.SyncCommitteeMessage{msg2, msg0, msg1}
	for _, msg := range msgs {
		require.NoError(t, proposerServer.SyncCommitteePool.SaveSyncCommitteeMessage(msg))
	}

	poolBits := qrysmpb.NewSyncCommitteeAggregationBits()
	poolBits.SetBitAt(4, true)
	lsig4, err := priv3.Sign([]byte("c4"))
	require.NoError(t, err)
	poolSig := lsig4.Marshal()
	contrib := &qrysmpb.SyncCommitteeContribution{
		Slot:              1,
		SubcommitteeIndex: 0,
		Signatures:        [][]byte{poolSig},
		AggregationBits:   poolBits,
		BlockRoot:         r[:],
	}
	require.NoError(t, proposerServer.SyncCommitteePool.SaveSyncCommitteeContribution(contrib))

	sa, err := proposerServer.getSyncAggregate(context.Background(), 1, r, st)
	require.NoError(t, err)
	assert.Equal(t, true, sa.SyncCommitteeBits.BitAt(0))
	assert.Equal(t, true, sa.SyncCommitteeBits.BitAt(1))
	assert.Equal(t, true, sa.SyncCommitteeBits.BitAt(2))
	assert.Equal(t, true, sa.SyncCommitteeBits.BitAt(3))
	assert.Equal(t, true, sa.SyncCommitteeBits.BitAt(4))
	require.DeepEqual(t, [][]byte{msg0.Signature, msg0.Signature, msg1.Signature, msg2.Signature, poolSig}, sa.SyncCommitteeSignatures)
}

func Test_aggregatedSyncCommitteeMessages_NoIntersectionWithPoolContributions(t *testing.T) {
	helpers.ClearCache()
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	priv0, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	priv1, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	priv2, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	priv3, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	pub0 := priv0.PublicKey().Marshal()
	pub1 := priv1.PublicKey().Marshal()
	pub2 := priv2.PublicKey().Marshal()
	pub3 := priv3.PublicKey().Marshal()

	vals := []*qrysmpb.Validator{
		{PublicKey: pub0},
		{PublicKey: pub1},
		{PublicKey: pub2},
		{PublicKey: pub3},
	}
	require.NoError(t, st.SetValidators(vals))

	sc := &qrysmpb.SyncCommittee{
		Pubkeys: make([][]byte, params.BeaconConfig().SyncCommitteeSize),
	}
	for i := range sc.Pubkeys {
		sc.Pubkeys[i] = make([]byte, len(pub0))
	}
	sc.Pubkeys[0] = pub0
	sc.Pubkeys[1] = pub1
	sc.Pubkeys[2] = pub2
	sc.Pubkeys[3] = pub3
	require.NoError(t, st.SetCurrentSyncCommittee(sc))

	proposerServer := &Server{
		HeadFetcher:       &mock.ChainService{State: st},
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		SyncCommitteePool: synccommittee.NewStore(),
	}

	r := params.BeaconConfig().ZeroHash
	sig0, err := priv0.Sign([]byte("m0"))
	require.NoError(t, err)
	sig1, err := priv1.Sign([]byte("m1"))
	require.NoError(t, err)
	sig2, err := priv2.Sign([]byte("m2"))
	require.NoError(t, err)
	sig3, err := priv3.Sign([]byte("m3"))
	require.NoError(t, err)
	msgs := []*qrysmpb.SyncCommitteeMessage{
		{Slot: 1, BlockRoot: r[:], ValidatorIndex: 0, Signature: sig0.Marshal()},
		{Slot: 1, BlockRoot: r[:], ValidatorIndex: 1, Signature: sig1.Marshal()},
		{Slot: 1, BlockRoot: r[:], ValidatorIndex: 2, Signature: sig2.Marshal()},
		{Slot: 1, BlockRoot: r[:], ValidatorIndex: 3, Signature: sig3.Marshal()},
	}
	for _, msg := range msgs {
		require.NoError(t, proposerServer.SyncCommitteePool.SaveSyncCommitteeMessage(msg))
	}

	poolBits := qrysmpb.NewSyncCommitteeAggregationBits()
	poolBits.SetBitAt(3, true)
	contSig, err := priv3.Sign([]byte("c3"))
	require.NoError(t, err)
	cont := &qrysmpb.SyncCommitteeContribution{
		Slot:              1,
		SubcommitteeIndex: 0,
		Signatures:        [][]byte{contSig.Marshal()},
		AggregationBits:   poolBits,
		BlockRoot:         r[:],
	}

	aggregated, err := proposerServer.aggregatedSyncCommitteeMessages(context.Background(), 1, r, []*qrysmpb.SyncCommitteeContribution{cont}, st)
	require.NoError(t, err)
	require.Equal(t, 1, len(aggregated))
	assert.Equal(t, false, aggregated[0].AggregationBits.BitAt(3))
}
