package beacon

import (
	"context"
	"testing"

	blockchainmock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	slashingsmock "github.com/theQRL/qrysm/beacon-chain/operations/slashings/mock"
	p2pMock "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/proto/migration"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestListPoolAttesterSlashings(t *testing.T) {
	bs, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	slashing1 := &qrysmpb.AttesterSlashing{
		Attestation_1: &qrysmpb.IndexedAttestation{
			AttestingIndices: []uint64{1, 10},
			Data: &qrysmpb.AttestationData{
				Slot:            1,
				CommitteeIndex:  1,
				BeaconBlockRoot: bytesutil.PadTo([]byte("blockroot1"), 32),
				Source: &qrysmpb.Checkpoint{
					Epoch: 1,
					Root:  bytesutil.PadTo([]byte("sourceroot1"), 32),
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 10,
					Root:  bytesutil.PadTo([]byte("targetroot1"), 32),
				},
			},
			Signatures: [][]byte{bytesutil.PadTo([]byte("signature1"), 4627)},
		},
		Attestation_2: &qrysmpb.IndexedAttestation{
			AttestingIndices: []uint64{2, 20},
			Data: &qrysmpb.AttestationData{
				Slot:            2,
				CommitteeIndex:  2,
				BeaconBlockRoot: bytesutil.PadTo([]byte("blockroot2"), 32),
				Source: &qrysmpb.Checkpoint{
					Epoch: 2,
					Root:  bytesutil.PadTo([]byte("sourceroot2"), 32),
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 20,
					Root:  bytesutil.PadTo([]byte("targetroot2"), 32),
				},
			},
			Signatures: [][]byte{bytesutil.PadTo([]byte("signature2"), 4627)},
		},
	}
	slashing2 := &qrysmpb.AttesterSlashing{
		Attestation_1: &qrysmpb.IndexedAttestation{
			AttestingIndices: []uint64{3, 30},
			Data: &qrysmpb.AttestationData{
				Slot:            3,
				CommitteeIndex:  3,
				BeaconBlockRoot: bytesutil.PadTo([]byte("blockroot3"), 32),
				Source: &qrysmpb.Checkpoint{
					Epoch: 3,
					Root:  bytesutil.PadTo([]byte("sourceroot3"), 32),
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 30,
					Root:  bytesutil.PadTo([]byte("targetroot3"), 32),
				},
			},
			Signatures: [][]byte{bytesutil.PadTo([]byte("signature3"), 4627)},
		},
		Attestation_2: &qrysmpb.IndexedAttestation{
			AttestingIndices: []uint64{4, 40},
			Data: &qrysmpb.AttestationData{
				Slot:            4,
				CommitteeIndex:  4,
				BeaconBlockRoot: bytesutil.PadTo([]byte("blockroot4"), 32),
				Source: &qrysmpb.Checkpoint{
					Epoch: 4,
					Root:  bytesutil.PadTo([]byte("sourceroot4"), 32),
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 40,
					Root:  bytesutil.PadTo([]byte("targetroot4"), 32),
				},
			},
			Signatures: [][]byte{bytesutil.PadTo([]byte("signature4"), 4627)},
		},
	}

	s := &Server{
		ChainInfoFetcher: &blockchainmock.ChainService{State: bs},
		SlashingsPool:    &slashingsmock.PoolMock{PendingAttSlashings: []*qrysmpb.AttesterSlashing{slashing1, slashing2}},
	}

	resp, err := s.ListPoolAttesterSlashings(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.Data))
	assert.DeepEqual(t, migration.V1Alpha1AttSlashingToV1(slashing1), resp.Data[0])
	assert.DeepEqual(t, migration.V1Alpha1AttSlashingToV1(slashing2), resp.Data[1])
}

func TestListPoolProposerSlashings(t *testing.T) {
	bs, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	slashing1 := &qrysmpb.ProposerSlashing{
		Header_1: &qrysmpb.SignedBeaconBlockHeader{
			Header: &qrysmpb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: 1,
				ParentRoot:    bytesutil.PadTo([]byte("parentroot1"), 32),
				StateRoot:     bytesutil.PadTo([]byte("stateroot1"), 32),
				BodyRoot:      bytesutil.PadTo([]byte("bodyroot1"), 32),
			},
			Signature: bytesutil.PadTo([]byte("signature1"), 96),
		},
		Header_2: &qrysmpb.SignedBeaconBlockHeader{
			Header: &qrysmpb.BeaconBlockHeader{
				Slot:          2,
				ProposerIndex: 2,
				ParentRoot:    bytesutil.PadTo([]byte("parentroot2"), 32),
				StateRoot:     bytesutil.PadTo([]byte("stateroot2"), 32),
				BodyRoot:      bytesutil.PadTo([]byte("bodyroot2"), 32),
			},
			Signature: bytesutil.PadTo([]byte("signature2"), 96),
		},
	}
	slashing2 := &qrysmpb.ProposerSlashing{
		Header_1: &qrysmpb.SignedBeaconBlockHeader{
			Header: &qrysmpb.BeaconBlockHeader{
				Slot:          3,
				ProposerIndex: 3,
				ParentRoot:    bytesutil.PadTo([]byte("parentroot3"), 32),
				StateRoot:     bytesutil.PadTo([]byte("stateroot3"), 32),
				BodyRoot:      bytesutil.PadTo([]byte("bodyroot3"), 32),
			},
			Signature: bytesutil.PadTo([]byte("signature3"), 96),
		},
		Header_2: &qrysmpb.SignedBeaconBlockHeader{
			Header: &qrysmpb.BeaconBlockHeader{
				Slot:          4,
				ProposerIndex: 4,
				ParentRoot:    bytesutil.PadTo([]byte("parentroot4"), 32),
				StateRoot:     bytesutil.PadTo([]byte("stateroot4"), 32),
				BodyRoot:      bytesutil.PadTo([]byte("bodyroot4"), 32),
			},
			Signature: bytesutil.PadTo([]byte("signature4"), 96),
		},
	}

	s := &Server{
		ChainInfoFetcher: &blockchainmock.ChainService{State: bs},
		SlashingsPool:    &slashingsmock.PoolMock{PendingPropSlashings: []*qrysmpb.ProposerSlashing{slashing1, slashing2}},
	}

	resp, err := s.ListPoolProposerSlashings(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.Data))
	assert.DeepEqual(t, migration.V1Alpha1ProposerSlashingToV1(slashing1), resp.Data[0])
	assert.DeepEqual(t, migration.V1Alpha1ProposerSlashingToV1(slashing2), resp.Data[1])
}

func TestSubmitAttesterSlashing_Ok(t *testing.T) {
	ctx := context.Background()

	transition.SkipSlotCache.Disable()
	defer transition.SkipSlotCache.Enable()

	_, keys, err := util.DeterministicDepositsAndKeys(1)
	require.NoError(t, err)
	validator := &qrysmpb.Validator{
		PublicKey: keys[0].PublicKey().Marshal(),
	}
	bs, err := util.NewBeaconStateZond(func(state *qrysmpb.BeaconStateZond) error {
		state.Validators = []*qrysmpb.Validator{validator}
		return nil
	})
	require.NoError(t, err)

	slashing := &qrlpb.AttesterSlashing{
		Attestation_1: &qrlpb.IndexedAttestation{
			AttestingIndices: []uint64{0},
			Data: &qrlpb.AttestationData{
				Slot:            1,
				Index:           1,
				BeaconBlockRoot: bytesutil.PadTo([]byte("blockroot1"), 32),
				Source: &qrlpb.Checkpoint{
					Epoch: 1,
					Root:  bytesutil.PadTo([]byte("sourceroot1"), 32),
				},
				Target: &qrlpb.Checkpoint{
					Epoch: 10,
					Root:  bytesutil.PadTo([]byte("targetroot1"), 32),
				},
			},
			Signatures: [][]byte{make([]byte, 4627)},
		},
		Attestation_2: &qrlpb.IndexedAttestation{
			AttestingIndices: []uint64{0},
			Data: &qrlpb.AttestationData{
				Slot:            1,
				Index:           1,
				BeaconBlockRoot: bytesutil.PadTo([]byte("blockroot2"), 32),
				Source: &qrlpb.Checkpoint{
					Epoch: 1,
					Root:  bytesutil.PadTo([]byte("sourceroot2"), 32),
				},
				Target: &qrlpb.Checkpoint{
					Epoch: 10,
					Root:  bytesutil.PadTo([]byte("targetroot2"), 32),
				},
			},
			Signatures: [][]byte{make([]byte, 4627)},
		},
	}

	for _, att := range []*qrlpb.IndexedAttestation{slashing.Attestation_1, slashing.Attestation_2} {
		sb, err := signing.ComputeDomainAndSign(bs, att.Data.Target.Epoch, att.Data, params.BeaconConfig().DomainBeaconAttester, keys[0])
		require.NoError(t, err)
		sig, err := ml_dsa_87.SignatureFromBytes(sb)
		require.NoError(t, err)
		att.Signatures = [][]byte{sig.Marshal()}
	}

	broadcaster := &p2pMock.MockBroadcaster{}
	s := &Server{
		ChainInfoFetcher: &blockchainmock.ChainService{State: bs},
		SlashingsPool:    &slashingsmock.PoolMock{},
		Broadcaster:      broadcaster,
	}

	_, err = s.SubmitAttesterSlashing(ctx, slashing)
	require.NoError(t, err)
	pendingSlashings := s.SlashingsPool.PendingAttesterSlashings(ctx, bs, true)
	require.Equal(t, 1, len(pendingSlashings))
	assert.DeepEqual(t, migration.V1AttSlashingToV1Alpha1(slashing), pendingSlashings[0])
	assert.Equal(t, true, broadcaster.BroadcastCalled)
	require.Equal(t, 1, len(broadcaster.BroadcastMessages))
	_, ok := broadcaster.BroadcastMessages[0].(*qrysmpb.AttesterSlashing)
	assert.Equal(t, true, ok)
}

func TestSubmitAttesterSlashing_AcrossFork(t *testing.T) {
	ctx := context.Background()

	transition.SkipSlotCache.Disable()
	defer transition.SkipSlotCache.Enable()

	params.SetupTestConfigCleanup(t)

	bs, keys := util.DeterministicGenesisStateZond(t, 1)

	slashing := &qrlpb.AttesterSlashing{
		Attestation_1: &qrlpb.IndexedAttestation{
			AttestingIndices: []uint64{0},
			Data: &qrlpb.AttestationData{
				Slot:            params.BeaconConfig().SlotsPerEpoch,
				Index:           1,
				BeaconBlockRoot: bytesutil.PadTo([]byte("blockroot1"), 32),
				Source: &qrlpb.Checkpoint{
					Epoch: 1,
					Root:  bytesutil.PadTo([]byte("sourceroot1"), 32),
				},
				Target: &qrlpb.Checkpoint{
					Epoch: 10,
					Root:  bytesutil.PadTo([]byte("targetroot1"), 32),
				},
			},
			Signatures: [][]byte{make([]byte, 96)},
		},
		Attestation_2: &qrlpb.IndexedAttestation{
			AttestingIndices: []uint64{0},
			Data: &qrlpb.AttestationData{
				Slot:            params.BeaconConfig().SlotsPerEpoch,
				Index:           1,
				BeaconBlockRoot: bytesutil.PadTo([]byte("blockroot2"), 32),
				Source: &qrlpb.Checkpoint{
					Epoch: 1,
					Root:  bytesutil.PadTo([]byte("sourceroot2"), 32),
				},
				Target: &qrlpb.Checkpoint{
					Epoch: 10,
					Root:  bytesutil.PadTo([]byte("targetroot2"), 32),
				},
			},
			Signatures: [][]byte{make([]byte, 4627)},
		},
	}

	newBs := bs.Copy()
	newBs, err := transition.ProcessSlots(ctx, newBs, params.BeaconConfig().SlotsPerEpoch)
	require.NoError(t, err)

	for _, att := range []*qrlpb.IndexedAttestation{slashing.Attestation_1, slashing.Attestation_2} {
		sb, err := signing.ComputeDomainAndSign(newBs, att.Data.Target.Epoch, att.Data, params.BeaconConfig().DomainBeaconAttester, keys[0])
		require.NoError(t, err)
		sig, err := ml_dsa_87.SignatureFromBytes(sb)
		require.NoError(t, err)
		att.Signatures = [][]byte{sig.Marshal()}
	}

	broadcaster := &p2pMock.MockBroadcaster{}
	s := &Server{
		ChainInfoFetcher: &blockchainmock.ChainService{State: bs},
		SlashingsPool:    &slashingsmock.PoolMock{},
		Broadcaster:      broadcaster,
	}

	_, err = s.SubmitAttesterSlashing(ctx, slashing)
	require.NoError(t, err)
	pendingSlashings := s.SlashingsPool.PendingAttesterSlashings(ctx, bs, true)
	require.Equal(t, 1, len(pendingSlashings))
	assert.DeepEqual(t, migration.V1AttSlashingToV1Alpha1(slashing), pendingSlashings[0])
	assert.Equal(t, true, broadcaster.BroadcastCalled)
	require.Equal(t, 1, len(broadcaster.BroadcastMessages))
	_, ok := broadcaster.BroadcastMessages[0].(*qrysmpb.AttesterSlashing)
	assert.Equal(t, true, ok)
}

func TestSubmitAttesterSlashing_InvalidSlashing(t *testing.T) {
	ctx := context.Background()

	transition.SkipSlotCache.Disable()
	defer transition.SkipSlotCache.Enable()

	bs, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	attestation := &qrlpb.IndexedAttestation{
		AttestingIndices: []uint64{0},
		Data: &qrlpb.AttestationData{
			Slot:            1,
			Index:           1,
			BeaconBlockRoot: bytesutil.PadTo([]byte("blockroot1"), 32),
			Source: &qrlpb.Checkpoint{
				Epoch: 1,
				Root:  bytesutil.PadTo([]byte("sourceroot1"), 32),
			},
			Target: &qrlpb.Checkpoint{
				Epoch: 10,
				Root:  bytesutil.PadTo([]byte("targetroot1"), 32),
			},
		},
		Signatures: [][]byte{make([]byte, 96)},
	}

	slashing := &qrlpb.AttesterSlashing{
		Attestation_1: attestation,
		Attestation_2: attestation,
	}

	broadcaster := &p2pMock.MockBroadcaster{}
	s := &Server{
		ChainInfoFetcher: &blockchainmock.ChainService{State: bs},
		SlashingsPool:    &slashingsmock.PoolMock{},
		Broadcaster:      broadcaster,
	}

	_, err = s.SubmitAttesterSlashing(ctx, slashing)
	require.ErrorContains(t, "Invalid attester slashing", err)
	assert.Equal(t, false, broadcaster.BroadcastCalled)
}

func TestSubmitProposerSlashing_Ok(t *testing.T) {
	ctx := context.Background()

	transition.SkipSlotCache.Disable()
	defer transition.SkipSlotCache.Enable()

	_, keys, err := util.DeterministicDepositsAndKeys(1)
	require.NoError(t, err)
	validator := &qrysmpb.Validator{
		PublicKey:         keys[0].PublicKey().Marshal(),
		WithdrawableEpoch: primitives.Epoch(1),
	}
	bs, err := util.NewBeaconStateZond(func(state *qrysmpb.BeaconStateZond) error {
		state.Validators = []*qrysmpb.Validator{validator}
		return nil
	})
	require.NoError(t, err)

	slashing := &qrlpb.ProposerSlashing{
		SignedHeader_1: &qrlpb.SignedBeaconBlockHeader{
			Message: &qrlpb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: 0,
				ParentRoot:    bytesutil.PadTo([]byte("parentroot1"), 32),
				StateRoot:     bytesutil.PadTo([]byte("stateroot1"), 32),
				BodyRoot:      bytesutil.PadTo([]byte("bodyroot1"), 32),
			},
			Signature: make([]byte, 96),
		},
		SignedHeader_2: &qrlpb.SignedBeaconBlockHeader{
			Message: &qrlpb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: 0,
				ParentRoot:    bytesutil.PadTo([]byte("parentroot2"), 32),
				StateRoot:     bytesutil.PadTo([]byte("stateroot2"), 32),
				BodyRoot:      bytesutil.PadTo([]byte("bodyroot2"), 32),
			},
			Signature: make([]byte, 96),
		},
	}

	for _, h := range []*qrlpb.SignedBeaconBlockHeader{slashing.SignedHeader_1, slashing.SignedHeader_2} {
		sb, err := signing.ComputeDomainAndSign(
			bs,
			slots.ToEpoch(h.Message.Slot),
			h.Message,
			params.BeaconConfig().DomainBeaconProposer,
			keys[0],
		)
		require.NoError(t, err)
		sig, err := ml_dsa_87.SignatureFromBytes(sb)
		require.NoError(t, err)
		h.Signature = sig.Marshal()
	}

	broadcaster := &p2pMock.MockBroadcaster{}
	s := &Server{
		ChainInfoFetcher: &blockchainmock.ChainService{State: bs},
		SlashingsPool:    &slashingsmock.PoolMock{},
		Broadcaster:      broadcaster,
	}

	_, err = s.SubmitProposerSlashing(ctx, slashing)
	require.NoError(t, err)
	pendingSlashings := s.SlashingsPool.PendingProposerSlashings(ctx, bs, true)
	require.Equal(t, 1, len(pendingSlashings))
	assert.DeepEqual(t, migration.V1ProposerSlashingToV1Alpha1(slashing), pendingSlashings[0])
	assert.Equal(t, true, broadcaster.BroadcastCalled)
	require.Equal(t, 1, len(broadcaster.BroadcastMessages))
	_, ok := broadcaster.BroadcastMessages[0].(*qrysmpb.ProposerSlashing)
	assert.Equal(t, true, ok)
}

func TestSubmitProposerSlashing_AcrossFork(t *testing.T) {
	ctx := context.Background()

	transition.SkipSlotCache.Disable()
	defer transition.SkipSlotCache.Enable()

	params.SetupTestConfigCleanup(t)

	bs, keys := util.DeterministicGenesisStateZond(t, 1)

	slashing := &qrlpb.ProposerSlashing{
		SignedHeader_1: &qrlpb.SignedBeaconBlockHeader{
			Message: &qrlpb.BeaconBlockHeader{
				Slot:          params.BeaconConfig().SlotsPerEpoch,
				ProposerIndex: 0,
				ParentRoot:    bytesutil.PadTo([]byte("parentroot1"), 32),
				StateRoot:     bytesutil.PadTo([]byte("stateroot1"), 32),
				BodyRoot:      bytesutil.PadTo([]byte("bodyroot1"), 32),
			},
			Signature: make([]byte, 96),
		},
		SignedHeader_2: &qrlpb.SignedBeaconBlockHeader{
			Message: &qrlpb.BeaconBlockHeader{
				Slot:          params.BeaconConfig().SlotsPerEpoch,
				ProposerIndex: 0,
				ParentRoot:    bytesutil.PadTo([]byte("parentroot2"), 32),
				StateRoot:     bytesutil.PadTo([]byte("stateroot2"), 32),
				BodyRoot:      bytesutil.PadTo([]byte("bodyroot2"), 32),
			},
			Signature: make([]byte, 96),
		},
	}

	newBs := bs.Copy()
	newBs, err := transition.ProcessSlots(ctx, newBs, params.BeaconConfig().SlotsPerEpoch)
	require.NoError(t, err)

	for _, h := range []*qrlpb.SignedBeaconBlockHeader{slashing.SignedHeader_1, slashing.SignedHeader_2} {
		sb, err := signing.ComputeDomainAndSign(
			newBs,
			slots.ToEpoch(h.Message.Slot),
			h.Message,
			params.BeaconConfig().DomainBeaconProposer,
			keys[0],
		)
		require.NoError(t, err)
		sig, err := ml_dsa_87.SignatureFromBytes(sb)
		require.NoError(t, err)
		h.Signature = sig.Marshal()
	}

	broadcaster := &p2pMock.MockBroadcaster{}
	s := &Server{
		ChainInfoFetcher: &blockchainmock.ChainService{State: bs},
		SlashingsPool:    &slashingsmock.PoolMock{},
		Broadcaster:      broadcaster,
	}

	_, err = s.SubmitProposerSlashing(ctx, slashing)
	require.NoError(t, err)
	pendingSlashings := s.SlashingsPool.PendingProposerSlashings(ctx, bs, true)
	require.Equal(t, 1, len(pendingSlashings))
	assert.DeepEqual(t, migration.V1ProposerSlashingToV1Alpha1(slashing), pendingSlashings[0])
	assert.Equal(t, true, broadcaster.BroadcastCalled)
	require.Equal(t, 1, len(broadcaster.BroadcastMessages))
	_, ok := broadcaster.BroadcastMessages[0].(*qrysmpb.ProposerSlashing)
	assert.Equal(t, true, ok)
}

func TestSubmitProposerSlashing_InvalidSlashing(t *testing.T) {
	ctx := context.Background()

	transition.SkipSlotCache.Disable()
	defer transition.SkipSlotCache.Enable()

	bs, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	header := &qrlpb.SignedBeaconBlockHeader{
		Message: &qrlpb.BeaconBlockHeader{
			Slot:          1,
			ProposerIndex: 0,
			ParentRoot:    bytesutil.PadTo([]byte("parentroot1"), 32),
			StateRoot:     bytesutil.PadTo([]byte("stateroot1"), 32),
			BodyRoot:      bytesutil.PadTo([]byte("bodyroot1"), 32),
		},
		Signature: make([]byte, 96),
	}

	slashing := &qrlpb.ProposerSlashing{
		SignedHeader_1: header,
		SignedHeader_2: header,
	}

	broadcaster := &p2pMock.MockBroadcaster{}
	s := &Server{
		ChainInfoFetcher: &blockchainmock.ChainService{State: bs},
		SlashingsPool:    &slashingsmock.PoolMock{},
		Broadcaster:      broadcaster,
	}

	_, err = s.SubmitProposerSlashing(ctx, slashing)
	require.ErrorContains(t, "Invalid proposer slashing", err)
	assert.Equal(t, false, broadcaster.BroadcastCalled)
}
