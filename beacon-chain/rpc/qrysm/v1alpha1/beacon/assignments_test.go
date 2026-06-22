package beacon

import (
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"testing"

	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	dbTest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	mockstategen "github.com/theQRL/qrysm/beacon-chain/state/stategen/mock"
	"github.com/theQRL/qrysm/cmd"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
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
		&qrysmpb.ListValidatorAssignmentsRequest{
			QueryFilter: &qrysmpb.ListValidatorAssignmentsRequest_Epoch{
				Epoch: slots.ToEpoch(bs.GenesisTimeFetcher.CurrentSlot()) + 1,
			},
		},
	)
	assert.ErrorContains(t, wanted, err)
}

func TestServer_ListAssignments_Pagination_InputOutOfRange(t *testing.T) {
	helpers.ClearCache()
	db := dbTest.SetupDB(t)
	ctx := context.Background()
	count := 100
	validators := make([]*qrysmpb.Validator, 0, count)
	for i := range count {
		pubKey := make([]byte, field_params.MLDSA87PubkeyLength)
		withdrawalCred := make([]byte, field_params.WithdrawalCredentialsLength)
		binary.LittleEndian.PutUint64(pubKey, uint64(i))
		validators = append(validators, &qrysmpb.Validator{
			PublicKey:             pubKey,
			WithdrawalCredentials: withdrawalCred,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
			ActivationEpoch:       0,
		})
	}

	blk := util.NewBeaconBlockZond()
	blockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)

	s, err := util.NewBeaconStateZond()
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
			FinalizedCheckPoint: &qrysmpb.Checkpoint{
				Epoch: 0,
			},
		},
		GenesisTimeFetcher: &mock.ChainService{},
		StateGen:           stategen.New(db, doublylinkedtree.New()),
		ReplayerBuilder:    mockstategen.NewMockReplayerBuilder(mockstategen.WithMockState(s)),
	}

	wanted := fmt.Sprintf("page start %d >= list %d", 500, count)
	_, err = bs.ListValidatorAssignments(context.Background(), &qrysmpb.ListValidatorAssignmentsRequest{
		PageToken:   strconv.Itoa(2),
		QueryFilter: &qrysmpb.ListValidatorAssignmentsRequest_Genesis{Genesis: true},
	})
	assert.ErrorContains(t, wanted, err)
}

func TestServer_ListAssignments_Pagination_ExceedsMaxPageSize(t *testing.T) {
	bs := &Server{}
	exceedsMax := int32(cmd.Get().MaxRPCPageSize + 1)

	wanted := fmt.Sprintf("Requested page size %d can not be greater than max size %d", exceedsMax, cmd.Get().MaxRPCPageSize)
	req := &qrysmpb.ListValidatorAssignmentsRequest{
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
	validators := make([]*qrysmpb.Validator, 0, count)
	for i := range count {
		pubKey := make([]byte, field_params.MLDSA87PubkeyLength)
		withdrawalCred := make([]byte, field_params.WithdrawalCredentialsLength)
		binary.LittleEndian.PutUint64(pubKey, uint64(i))
		// Mark the validators with index divisible by 3 inactive.
		if i%3 == 0 {
			validators = append(validators, &qrysmpb.Validator{
				PublicKey:             pubKey,
				WithdrawalCredentials: withdrawalCred,
				ExitEpoch:             0,
				ActivationEpoch:       0,
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
			})
		} else {
			validators = append(validators, &qrysmpb.Validator{
				PublicKey:             pubKey,
				WithdrawalCredentials: withdrawalCred,
				ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
				ActivationEpoch:       0,
			})
		}
	}

	b := util.NewBeaconBlockZond()
	blockRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)

	s, err := util.NewBeaconStateZond()
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
			FinalizedCheckPoint: &qrysmpb.Checkpoint{
				Epoch: 0,
			},
		},
		GenesisTimeFetcher: &mock.ChainService{},
		StateGen:           stategen.New(db, doublylinkedtree.New()),
		ReplayerBuilder:    mockstategen.NewMockReplayerBuilder(mockstategen.WithMockState(s)),
	}

	res, err := bs.ListValidatorAssignments(context.Background(), &qrysmpb.ListValidatorAssignmentsRequest{
		QueryFilter: &qrysmpb.ListValidatorAssignmentsRequest_Genesis{Genesis: true},
	})
	require.NoError(t, err)

	// Construct the wanted assignments.
	var wanted []*qrysmpb.ValidatorAssignments_CommitteeAssignment

	activeIndices, err := helpers.ActiveValidatorIndices(ctx, s, 0)
	require.NoError(t, err)
	committeeAssignments, err := helpers.CommitteeAssignments(context.Background(), s, 0, activeIndices[0:params.BeaconConfig().DefaultPageSize])
	require.NoError(t, err)
	proposerIndexToSlots, err := helpers.ProposerAssignments(context.Background(), s, 0)
	require.NoError(t, err)
	for _, index := range activeIndices[0:params.BeaconConfig().DefaultPageSize] {
		require.NoError(t, err)
		wanted = append(wanted, &qrysmpb.ValidatorAssignments_CommitteeAssignment{
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
	validators := make([]*qrysmpb.Validator, 0, count)
	withdrawCreds := make([]byte, field_params.WithdrawalCredentialsLength)
	for i := range count {
		pubKey := make([]byte, field_params.MLDSA87PubkeyLength)
		binary.LittleEndian.PutUint64(pubKey, uint64(i))
		val := &qrysmpb.Validator{
			PublicKey:             pubKey,
			WithdrawalCredentials: withdrawCreds,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		}
		validators = append(validators, val)
	}

	b := util.NewBeaconBlockZond()
	blockRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, s.SetValidators(validators))
	require.NoError(t, db.SaveState(ctx, s, blockRoot))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, blockRoot))

	bs := &Server{
		BeaconDB: db,
		FinalizationFetcher: &mock.ChainService{
			FinalizedCheckPoint: &qrysmpb.Checkpoint{
				Epoch: 0,
			},
		},
		GenesisTimeFetcher: &mock.ChainService{},
		StateGen:           stategen.New(db, doublylinkedtree.New()),
		ReplayerBuilder:    mockstategen.NewMockReplayerBuilder(mockstategen.WithMockState(s)),
	}

	pubKey1 := make([]byte, field_params.MLDSA87PubkeyLength)
	binary.LittleEndian.PutUint64(pubKey1, 1)
	pubKey2 := make([]byte, field_params.MLDSA87PubkeyLength)
	binary.LittleEndian.PutUint64(pubKey2, 2)
	req := &qrysmpb.ListValidatorAssignmentsRequest{PublicKeys: [][]byte{pubKey1, pubKey2}, Indices: []primitives.ValidatorIndex{2, 3}}
	res, err := bs.ListValidatorAssignments(context.Background(), req)
	require.NoError(t, err)

	// Construct the wanted assignments.
	var wanted []*qrysmpb.ValidatorAssignments_CommitteeAssignment

	activeIndices, err := helpers.ActiveValidatorIndices(ctx, s, 0)
	require.NoError(t, err)
	committeeAssignments, err := helpers.CommitteeAssignments(context.Background(), s, 0, activeIndices[1:4])
	require.NoError(t, err)
	proposerIndexToSlots, err := helpers.ProposerAssignments(context.Background(), s, 0)
	require.NoError(t, err)
	for _, index := range activeIndices[1:4] {
		require.NoError(t, err)
		wanted = append(wanted, &qrysmpb.ValidatorAssignments_CommitteeAssignment{
			BeaconCommittees: committeeAssignments[index].Committee,
			CommitteeIndex:   committeeAssignments[index].CommitteeIndex,
			AttesterSlot:     committeeAssignments[index].AttesterSlot,
			ProposerSlots:    proposerIndexToSlots[index],
			ValidatorIndex:   index,
		})
	}

	assert.DeepEqual(t, wanted, res.Assignments, "Did not receive wanted assignments")
}

// TestServer_ListAssignments_HandlesUnscheduledValidator is the regression
// test for the nil-deref fix analogous to upstream PR #15466. When req.Indices
// includes a validator that has no committee assignment in the requested epoch
// (e.g. inactive / exited), CommitteeAssignments returns no entry for it.
// Before the fix, ListValidatorAssignments dereferenced the missing entry and
// panicked; after the fix, it returns an assignment with zero-valued committee
// fields.
func TestServer_ListAssignments_HandlesUnscheduledValidator(t *testing.T) {
	helpers.ClearCache()
	db := dbTest.SetupDB(t)
	ctx := context.Background()
	count := 64
	validators := make([]*qrysmpb.Validator, 0, count)
	withdrawCreds := make([]byte, field_params.WithdrawalCredentialsLength)
	for i := range count {
		pubKey := make([]byte, field_params.MLDSA87PubkeyLength)
		binary.LittleEndian.PutUint64(pubKey, uint64(i))
		val := &qrysmpb.Validator{
			PublicKey:             pubKey,
			WithdrawalCredentials: withdrawCreds,
			ActivationEpoch:       0,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
		}
		// Mark index 0 as already-exited so it has no committee assignment.
		if i == 0 {
			val.ExitEpoch = 0
		}
		validators = append(validators, val)
	}

	b := util.NewBeaconBlockZond()
	blockRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, s.SetValidators(validators))
	require.NoError(t, db.SaveState(ctx, s, blockRoot))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, blockRoot))

	bs := &Server{
		BeaconDB: db,
		FinalizationFetcher: &mock.ChainService{
			FinalizedCheckPoint: &qrysmpb.Checkpoint{Epoch: 0},
		},
		GenesisTimeFetcher: &mock.ChainService{},
		StateGen:           stategen.New(db, doublylinkedtree.New()),
		ReplayerBuilder:    mockstategen.NewMockReplayerBuilder(mockstategen.WithMockState(s)),
	}

	// Request the exited validator's index — must not panic.
	req := &qrysmpb.ListValidatorAssignmentsRequest{Indices: []primitives.ValidatorIndex{0}}
	res, err := bs.ListValidatorAssignments(ctx, req)
	require.NoError(t, err)
	require.Equal(t, 1, len(res.Assignments))
	got := res.Assignments[0]
	require.Equal(t, primitives.ValidatorIndex(0), got.ValidatorIndex)
	require.Equal(t, 0, len(got.BeaconCommittees), "unscheduled validator should have empty committee")
	require.Equal(t, primitives.Slot(0), got.AttesterSlot)
	require.Equal(t, primitives.CommitteeIndex(0), got.CommitteeIndex)
}

func TestServer_ListAssignments_CanFilterPubkeysIndices_WithPagination(t *testing.T) {
	helpers.ClearCache()
	db := dbTest.SetupDB(t)
	ctx := context.Background()
	count := 100
	validators := make([]*qrysmpb.Validator, 0, count)
	withdrawCred := make([]byte, field_params.WithdrawalCredentialsLength)
	for i := range count {
		pubKey := make([]byte, field_params.MLDSA87PubkeyLength)
		binary.LittleEndian.PutUint64(pubKey, uint64(i))
		val := &qrysmpb.Validator{
			PublicKey:             pubKey,
			WithdrawalCredentials: withdrawCred,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		}
		validators = append(validators, val)
	}

	b := util.NewBeaconBlockZond()
	blockRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, db, b)
	require.NoError(t, s.SetValidators(validators))
	require.NoError(t, db.SaveState(ctx, s, blockRoot))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, blockRoot))

	bs := &Server{
		BeaconDB: db,
		FinalizationFetcher: &mock.ChainService{
			FinalizedCheckPoint: &qrysmpb.Checkpoint{
				Epoch: 0,
			},
		},
		GenesisTimeFetcher: &mock.ChainService{},
		StateGen:           stategen.New(db, doublylinkedtree.New()),
	}

	addDefaultReplayerBuilder(bs, db)

	req := &qrysmpb.ListValidatorAssignmentsRequest{Indices: []primitives.ValidatorIndex{1, 2, 3, 4, 5, 6}, PageSize: 2, PageToken: "1"}
	res, err := bs.ListValidatorAssignments(context.Background(), req)
	require.NoError(t, err)

	// Construct the wanted assignments.
	var assignments []*qrysmpb.ValidatorAssignments_CommitteeAssignment

	activeIndices, err := helpers.ActiveValidatorIndices(ctx, s, 0)
	require.NoError(t, err)
	committeeAssignments, err := helpers.CommitteeAssignments(context.Background(), s, 0, activeIndices[3:5])
	require.NoError(t, err)
	proposerIndexToSlots, err := helpers.ProposerAssignments(context.Background(), s, 0)
	require.NoError(t, err)
	for _, index := range activeIndices[3:5] {
		require.NoError(t, err)
		assignments = append(assignments, &qrysmpb.ValidatorAssignments_CommitteeAssignment{
			BeaconCommittees: committeeAssignments[index].Committee,
			CommitteeIndex:   committeeAssignments[index].CommitteeIndex,
			AttesterSlot:     committeeAssignments[index].AttesterSlot,
			ProposerSlots:    proposerIndexToSlots[index],
			ValidatorIndex:   index,
		})
	}

	wantedRes := &qrysmpb.ValidatorAssignments{
		Assignments:   assignments,
		TotalSize:     int32(len(req.Indices)),
		NextPageToken: "2",
	}

	assert.DeepEqual(t, wantedRes, res, "Did not get wanted assignments")

	// Test the wrap around scenario.
	assignments = nil
	req = &qrysmpb.ListValidatorAssignmentsRequest{Indices: []primitives.ValidatorIndex{1, 2, 3, 4, 5, 6}, PageSize: 5, PageToken: "1"}
	res, err = bs.ListValidatorAssignments(context.Background(), req)
	require.NoError(t, err)
	cAssignments, err := helpers.CommitteeAssignments(context.Background(), s, 0, activeIndices[6:7])
	require.NoError(t, err)
	proposerIndexToSlots, err = helpers.ProposerAssignments(context.Background(), s, 0)
	require.NoError(t, err)
	for _, index := range activeIndices[6:7] {
		require.NoError(t, err)
		assignments = append(assignments, &qrysmpb.ValidatorAssignments_CommitteeAssignment{
			BeaconCommittees: cAssignments[index].Committee,
			CommitteeIndex:   cAssignments[index].CommitteeIndex,
			AttesterSlot:     cAssignments[index].AttesterSlot,
			ProposerSlots:    proposerIndexToSlots[index],
			ValidatorIndex:   index,
		})
	}

	wantedRes = &qrysmpb.ValidatorAssignments{
		Assignments:   assignments,
		TotalSize:     int32(len(req.Indices)),
		NextPageToken: "",
	}

	assert.DeepEqual(t, wantedRes, res, "Did not receive wanted assignments")
}
