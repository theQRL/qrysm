package beacon

import (
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"testing"

	mock "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	dbTest "github.com/theQRL/qrysm/v4/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/v4/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/v4/beacon-chain/state/stategen"
	mockstategen "github.com/theQRL/qrysm/v4/beacon-chain/state/stategen/mock"
	"github.com/theQRL/qrysm/v4/cmd"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	"github.com/theQRL/qrysm/v4/time/slots"
	"google.golang.org/protobuf/proto"
)

func TestServer_ListAssignments_CannotRequestFutureEpoch(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()
	bs := &Server{
		BeaconDB:           db,
		GenesisTimeFetcher: &mock.ChainService{},
	}
	addDefaultReplayerBuilder(bs, db)

	wanted := errNoEpochInfoError
	_, err := bs.ListValidatorAssignments(
		ctx,
		&zondpb.ListValidatorAssignmentsRequest{
			QueryFilter: &zondpb.ListValidatorAssignmentsRequest_Epoch{
				Epoch: slots.ToEpoch(bs.GenesisTimeFetcher.CurrentSlot()) + 1,
			},
		},
	)
	assert.ErrorContains(t, wanted, err)
}

func TestServer_ListAssignments_NoResults(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()
	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)

	b := util.NewBeaconBlockCapella()
	util.SaveBlock(t, ctx, db, b)
	gRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, gRoot))
	require.NoError(t, db.SaveState(ctx, st, gRoot))

	bs := &Server{
		BeaconDB:           db,
		GenesisTimeFetcher: &mock.ChainService{},
		StateGen:           stategen.New(db, doublylinkedtree.New()),
		ReplayerBuilder:    mockstategen.NewMockReplayerBuilder(mockstategen.WithMockState(st)),
	}
	wanted := &zondpb.ValidatorAssignments{
		Assignments:   make([]*zondpb.ValidatorAssignments_CommitteeAssignment, 0),
		TotalSize:     int32(0),
		NextPageToken: strconv.Itoa(0),
	}
	res, err := bs.ListValidatorAssignments(
		ctx,
		&zondpb.ListValidatorAssignmentsRequest{
			QueryFilter: &zondpb.ListValidatorAssignmentsRequest_Genesis{
				Genesis: true,
			},
		},
	)
	require.NoError(t, err)
	if !proto.Equal(wanted, res) {
		t.Errorf("Wanted %v, received %v", wanted, res)
	}
}

func TestServer_ListAssignments_Pagination_InputOutOfRange(t *testing.T) {
	helpers.ClearCache()
	db := dbTest.SetupDB(t)
	ctx := context.Background()
	count := 100
	validators := make([]*zondpb.Validator, 0, count)
	for i := 0; i < count; i++ {
		pubKey := make([]byte, field_params.DilithiumPubkeyLength)
		withdrawalCred := make([]byte, 32)
		binary.LittleEndian.PutUint64(pubKey, uint64(i))
		validators = append(validators, &zondpb.Validator{
			PublicKey:             pubKey,
			WithdrawalCredentials: withdrawalCred,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
			ActivationEpoch:       0,
		})
	}

	blk := util.NewBeaconBlockCapella()
	blockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)

	s, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, s.SetValidators(validators))
	require.NoError(t, db.SaveState(ctx, s, blockRoot))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, blockRoot))

	bs := &Server{
		BeaconDB: db,
		HeadFetcher: &mock.ChainService{
			State: s,
		},
		FinalizationFetcher: &mock.ChainService{
			FinalizedCheckPoint: &zondpb.Checkpoint{
				Epoch: 0,
			},
		},
		GenesisTimeFetcher: &mock.ChainService{},
		StateGen:           stategen.New(db, doublylinkedtree.New()),
		ReplayerBuilder:    mockstategen.NewMockReplayerBuilder(mockstategen.WithMockState(s)),
	}

	wanted := fmt.Sprintf("page start %d >= list %d", 500, count)
	_, err = bs.ListValidatorAssignments(context.Background(), &zondpb.ListValidatorAssignmentsRequest{
		PageToken:   strconv.Itoa(2),
		QueryFilter: &zondpb.ListValidatorAssignmentsRequest_Genesis{Genesis: true},
	})
	assert.ErrorContains(t, wanted, err)
}

func TestServer_ListAssignments_Pagination_ExceedsMaxPageSize(t *testing.T) {
	bs := &Server{}
	exceedsMax := int32(cmd.Get().MaxRPCPageSize + 1)

	wanted := fmt.Sprintf("Requested page size %d can not be greater than max size %d", exceedsMax, cmd.Get().MaxRPCPageSize)
	req := &zondpb.ListValidatorAssignmentsRequest{
		PageToken: strconv.Itoa(0),
		PageSize:  exceedsMax,
	}
	_, err := bs.ListValidatorAssignments(context.Background(), req)
	assert.ErrorContains(t, wanted, err)
}

func TestServer_ListAssignments_Pagination_DefaultPageSize_NoArchive(t *testing.T) {
	helpers.ClearCache()
	db := dbTest.SetupDB(t)
	ctx := context.Background()
	count := 500
	validators := make([]*zondpb.Validator, 0, count)
	for i := 0; i < count; i++ {
		pubKey := make([]byte, field_params.DilithiumPubkeyLength)
		withdrawalCred := make([]byte, 32)
		binary.LittleEndian.PutUint64(pubKey, uint64(i))
		// Mark the validators with index divisible by 3 inactive.
		if i%3 == 0 {
			validators = append(validators, &zondpb.Validator{
				PublicKey:             pubKey,
				WithdrawalCredentials: withdrawalCred,
				ExitEpoch:             0,
				ActivationEpoch:       0,
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
			})
		} else {
			validators = append(validators, &zondpb.Validator{
				PublicKey:             pubKey,
				WithdrawalCredentials: withdrawalCred,
				ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
				ActivationEpoch:       0,
			})
		}
	}

	b := util.NewBeaconBlockCapella()
	blockRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)

	s, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, s.SetValidators(validators))
	require.NoError(t, db.SaveState(ctx, s, blockRoot))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, blockRoot))

	bs := &Server{
		BeaconDB: db,
		HeadFetcher: &mock.ChainService{
			State: s,
		},
		FinalizationFetcher: &mock.ChainService{
			FinalizedCheckPoint: &zondpb.Checkpoint{
				Epoch: 0,
			},
		},
		GenesisTimeFetcher: &mock.ChainService{},
		StateGen:           stategen.New(db, doublylinkedtree.New()),
		ReplayerBuilder:    mockstategen.NewMockReplayerBuilder(mockstategen.WithMockState(s)),
	}

	res, err := bs.ListValidatorAssignments(context.Background(), &zondpb.ListValidatorAssignmentsRequest{
		QueryFilter: &zondpb.ListValidatorAssignmentsRequest_Genesis{Genesis: true},
	})
	require.NoError(t, err)

	// Construct the wanted assignments.
	var wanted []*zondpb.ValidatorAssignments_CommitteeAssignment

	activeIndices, err := helpers.ActiveValidatorIndices(ctx, s, 0)
	require.NoError(t, err)
	committeeAssignments, proposerIndexToSlots, err := helpers.CommitteeAssignments(context.Background(), s, 0)
	require.NoError(t, err)
	for _, index := range activeIndices[0:params.BeaconConfig().DefaultPageSize] {
		require.NoError(t, err)
		wanted = append(wanted, &zondpb.ValidatorAssignments_CommitteeAssignment{
			BeaconCommittees: committeeAssignments[index].Committee,
			CommitteeIndex:   committeeAssignments[index].CommitteeIndex,
			AttesterSlot:     committeeAssignments[index].AttesterSlot,
			ProposerSlots:    proposerIndexToSlots[index],
			ValidatorIndex:   index,
		})
	}
	assert.DeepSSZEqual(t, wanted, res.Assignments, "Did not receive wanted assignments")
}

func TestServer_ListAssignments_FilterPubkeysIndices_NoPagination(t *testing.T) {
	helpers.ClearCache()
	db := dbTest.SetupDB(t)

	ctx := context.Background()
	count := 100
	validators := make([]*zondpb.Validator, 0, count)
	withdrawCreds := make([]byte, 32)
	for i := 0; i < count; i++ {
		pubKey := make([]byte, field_params.DilithiumPubkeyLength)
		binary.LittleEndian.PutUint64(pubKey, uint64(i))
		val := &zondpb.Validator{
			PublicKey:             pubKey,
			WithdrawalCredentials: withdrawCreds,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		}
		validators = append(validators, val)
	}

	b := util.NewBeaconBlockCapella()
	blockRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	s, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, s.SetValidators(validators))
	require.NoError(t, db.SaveState(ctx, s, blockRoot))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, blockRoot))

	bs := &Server{
		BeaconDB: db,
		FinalizationFetcher: &mock.ChainService{
			FinalizedCheckPoint: &zondpb.Checkpoint{
				Epoch: 0,
			},
		},
		GenesisTimeFetcher: &mock.ChainService{},
		StateGen:           stategen.New(db, doublylinkedtree.New()),
		ReplayerBuilder:    mockstategen.NewMockReplayerBuilder(mockstategen.WithMockState(s)),
	}

	pubKey1 := make([]byte, field_params.DilithiumPubkeyLength)
	binary.LittleEndian.PutUint64(pubKey1, 1)
	pubKey2 := make([]byte, field_params.DilithiumPubkeyLength)
	binary.LittleEndian.PutUint64(pubKey2, 2)
	req := &zondpb.ListValidatorAssignmentsRequest{PublicKeys: [][]byte{pubKey1, pubKey2}, Indices: []primitives.ValidatorIndex{2, 3}}
	res, err := bs.ListValidatorAssignments(context.Background(), req)
	require.NoError(t, err)

	// Construct the wanted assignments.
	var wanted []*zondpb.ValidatorAssignments_CommitteeAssignment

	activeIndices, err := helpers.ActiveValidatorIndices(ctx, s, 0)
	require.NoError(t, err)
	committeeAssignments, proposerIndexToSlots, err := helpers.CommitteeAssignments(context.Background(), s, 0)
	require.NoError(t, err)
	for _, index := range activeIndices[1:4] {
		require.NoError(t, err)
		wanted = append(wanted, &zondpb.ValidatorAssignments_CommitteeAssignment{
			BeaconCommittees: committeeAssignments[index].Committee,
			CommitteeIndex:   committeeAssignments[index].CommitteeIndex,
			AttesterSlot:     committeeAssignments[index].AttesterSlot,
			ProposerSlots:    proposerIndexToSlots[index],
			ValidatorIndex:   index,
		})
	}

	assert.DeepEqual(t, wanted, res.Assignments, "Did not receive wanted assignments")
}

func TestServer_ListAssignments_CanFilterPubkeysIndices_WithPagination(t *testing.T) {
	helpers.ClearCache()
	db := dbTest.SetupDB(t)
	ctx := context.Background()
	count := 100
	validators := make([]*zondpb.Validator, 0, count)
	withdrawCred := make([]byte, 32)
	for i := 0; i < count; i++ {
		pubKey := make([]byte, field_params.DilithiumPubkeyLength)
		binary.LittleEndian.PutUint64(pubKey, uint64(i))
		val := &zondpb.Validator{
			PublicKey:             pubKey,
			WithdrawalCredentials: withdrawCred,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		}
		validators = append(validators, val)
	}

	b := util.NewBeaconBlockCapella()
	blockRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	s, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, db, b)
	require.NoError(t, s.SetValidators(validators))
	require.NoError(t, db.SaveState(ctx, s, blockRoot))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, blockRoot))

	bs := &Server{
		BeaconDB: db,
		FinalizationFetcher: &mock.ChainService{
			FinalizedCheckPoint: &zondpb.Checkpoint{
				Epoch: 0,
			},
		},
		GenesisTimeFetcher: &mock.ChainService{},
		StateGen:           stategen.New(db, doublylinkedtree.New()),
	}

	addDefaultReplayerBuilder(bs, db)

	req := &zondpb.ListValidatorAssignmentsRequest{Indices: []primitives.ValidatorIndex{1, 2, 3, 4, 5, 6}, PageSize: 2, PageToken: "1"}
	res, err := bs.ListValidatorAssignments(context.Background(), req)
	require.NoError(t, err)

	// Construct the wanted assignments.
	var assignments []*zondpb.ValidatorAssignments_CommitteeAssignment

	activeIndices, err := helpers.ActiveValidatorIndices(ctx, s, 0)
	require.NoError(t, err)
	committeeAssignments, proposerIndexToSlots, err := helpers.CommitteeAssignments(context.Background(), s, 0)
	require.NoError(t, err)
	for _, index := range activeIndices[3:5] {
		require.NoError(t, err)
		assignments = append(assignments, &zondpb.ValidatorAssignments_CommitteeAssignment{
			BeaconCommittees: committeeAssignments[index].Committee,
			CommitteeIndex:   committeeAssignments[index].CommitteeIndex,
			AttesterSlot:     committeeAssignments[index].AttesterSlot,
			ProposerSlots:    proposerIndexToSlots[index],
			ValidatorIndex:   index,
		})
	}

	wantedRes := &zondpb.ValidatorAssignments{
		Assignments:   assignments,
		TotalSize:     int32(len(req.Indices)),
		NextPageToken: "2",
	}

	assert.DeepEqual(t, wantedRes, res, "Did not get wanted assignments")

	// Test the wrap around scenario.
	assignments = nil
	req = &zondpb.ListValidatorAssignmentsRequest{Indices: []primitives.ValidatorIndex{1, 2, 3, 4, 5, 6}, PageSize: 5, PageToken: "1"}
	res, err = bs.ListValidatorAssignments(context.Background(), req)
	require.NoError(t, err)
	cAssignments, proposerIndexToSlots, err := helpers.CommitteeAssignments(context.Background(), s, 0)
	require.NoError(t, err)
	for _, index := range activeIndices[6:7] {
		require.NoError(t, err)
		assignments = append(assignments, &zondpb.ValidatorAssignments_CommitteeAssignment{
			BeaconCommittees: cAssignments[index].Committee,
			CommitteeIndex:   cAssignments[index].CommitteeIndex,
			AttesterSlot:     cAssignments[index].AttesterSlot,
			ProposerSlots:    proposerIndexToSlots[index],
			ValidatorIndex:   index,
		})
	}

	wantedRes = &zondpb.ValidatorAssignments{
		Assignments:   assignments,
		TotalSize:     int32(len(req.Indices)),
		NextPageToken: "",
	}

	assert.DeepEqual(t, wantedRes, res, "Did not receive wanted assignments")
}
