package beacon

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/theQRL/qrysm/api/pagination"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/epoch/precompute"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	coreTime "github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/core/validators"
	"github.com/theQRL/qrysm/beacon-chain/rpc/core"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/cmd"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ListValidatorBalances retrieves the validator balances for a given set of public keys.
// An optional Epoch parameter is provided to request historical validator balances from
// archived, persistent data.
func (bs *Server) ListValidatorBalances(
	ctx context.Context,
	req *zondpb.ListValidatorBalancesRequest,
) (*zondpb.ValidatorBalances, error) {
	if int(req.PageSize) > cmd.Get().MaxRPCPageSize {
		return nil, status.Errorf(codes.InvalidArgument, "Requested page size %d can not be greater than max size %d",
			req.PageSize, cmd.Get().MaxRPCPageSize)
	}

	if bs.GenesisTimeFetcher == nil {
		return nil, status.Errorf(codes.Internal, "Nil genesis time fetcher")
	}
	currentEpoch := slots.ToEpoch(bs.GenesisTimeFetcher.CurrentSlot())
	requestedEpoch := currentEpoch
	switch q := req.QueryFilter.(type) {
	case *zondpb.ListValidatorBalancesRequest_Epoch:
		requestedEpoch = q.Epoch
	case *zondpb.ListValidatorBalancesRequest_Genesis:
		requestedEpoch = 0
	}

	if requestedEpoch > currentEpoch {
		return nil, status.Errorf(
			codes.InvalidArgument,
			errEpoch,
			currentEpoch,
			requestedEpoch,
		)
	}
	res := make([]*zondpb.ValidatorBalances_Balance, 0)
	filtered := map[primitives.ValidatorIndex]bool{} // Track filtered validators to prevent duplication in the response.

	startSlot, err := slots.EpochStart(requestedEpoch)
	if err != nil {
		return nil, err
	}
	requestedState, err := bs.ReplayerBuilder.ReplayerForSlot(startSlot).ReplayBlocks(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("error replaying blocks for state at slot %d: %v", startSlot, err))
	}

	vals := requestedState.Validators()
	balances := requestedState.Balances()
	balancesCount := len(balances)
	for _, pubKey := range req.PublicKeys {
		// Skip empty public key.
		if len(pubKey) == 0 {
			continue
		}
		pubkeyBytes := bytesutil.ToBytes2592(pubKey)
		index, ok := requestedState.ValidatorIndexByPubkey(pubkeyBytes)
		if !ok {
			// We continue the loop if one validator in the request is not found.
			res = append(res, &zondpb.ValidatorBalances_Balance{
				Status: "UNKNOWN",
			})
			balancesCount = len(res)
			continue
		}
		filtered[index] = true

		if uint64(index) >= uint64(len(balances)) {
			return nil, status.Errorf(codes.OutOfRange, "Validator index %d >= balance list %d",
				index, len(balances))
		}

		val := vals[index]
		st := validatorStatus(val, requestedEpoch)
		res = append(res, &zondpb.ValidatorBalances_Balance{
			PublicKey: pubKey,
			Index:     index,
			Balance:   balances[index],
			Status:    st.String(),
		})
		balancesCount = len(res)
	}

	for _, index := range req.Indices {
		if uint64(index) >= uint64(len(balances)) {
			return nil, status.Errorf(codes.OutOfRange, "Validator index %d >= balance list %d",
				index, len(balances))
		}

		if !filtered[index] {
			val := vals[index]
			st := validatorStatus(val, requestedEpoch)
			res = append(res, &zondpb.ValidatorBalances_Balance{
				PublicKey: vals[index].PublicKey,
				Index:     index,
				Balance:   balances[index],
				Status:    st.String(),
			})
		}
		balancesCount = len(res)
	}
	// Depending on the indices and public keys given, results might not be sorted.
	sort.Slice(res, func(i, j int) bool {
		return res[i].Index < res[j].Index
	})

	// If there are no balances, we simply return a response specifying this.
	// Otherwise, attempting to paginate 0 balances below would result in an error.
	if balancesCount == 0 {
		return &zondpb.ValidatorBalances{
			Epoch:         requestedEpoch,
			Balances:      make([]*zondpb.ValidatorBalances_Balance, 0),
			TotalSize:     int32(0),
			NextPageToken: strconv.Itoa(0),
		}, nil
	}

	start, end, nextPageToken, err := pagination.StartAndEndPage(req.PageToken, int(req.PageSize), balancesCount)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"Could not paginate results: %v",
			err,
		)
	}

	if len(req.Indices) == 0 && len(req.PublicKeys) == 0 {
		// Return everything.
		for i := start; i < end; i++ {
			pubkey := requestedState.PubkeyAtIndex(primitives.ValidatorIndex(i))
			val := vals[i]
			st := validatorStatus(val, requestedEpoch)
			res = append(res, &zondpb.ValidatorBalances_Balance{
				PublicKey: pubkey[:],
				Index:     primitives.ValidatorIndex(i),
				Balance:   balances[i],
				Status:    st.String(),
			})
		}
		return &zondpb.ValidatorBalances{
			Epoch:         requestedEpoch,
			Balances:      res,
			TotalSize:     int32(balancesCount),
			NextPageToken: nextPageToken,
		}, nil
	}

	if end > len(res) || end < start {
		return nil, status.Error(codes.OutOfRange, "Request exceeds response length")
	}

	return &zondpb.ValidatorBalances{
		Epoch:         requestedEpoch,
		Balances:      res[start:end],
		TotalSize:     int32(balancesCount),
		NextPageToken: nextPageToken,
	}, nil
}

// ListValidators retrieves the current list of active validators with an optional historical epoch flag to
// retrieve validator set in time.
func (bs *Server) ListValidators(
	ctx context.Context,
	req *zondpb.ListValidatorsRequest,
) (*zondpb.Validators, error) {
	if int(req.PageSize) > cmd.Get().MaxRPCPageSize {
		return nil, status.Errorf(codes.InvalidArgument, "Requested page size %d can not be greater than max size %d",
			req.PageSize, cmd.Get().MaxRPCPageSize)
	}

	currentEpoch := slots.ToEpoch(bs.GenesisTimeFetcher.CurrentSlot())
	requestedEpoch := currentEpoch

	switch q := req.QueryFilter.(type) {
	case *zondpb.ListValidatorsRequest_Genesis:
		if q.Genesis {
			requestedEpoch = 0
		}
	case *zondpb.ListValidatorsRequest_Epoch:
		if q.Epoch > currentEpoch {
			return nil, status.Errorf(
				codes.InvalidArgument,
				errEpoch,
				currentEpoch,
				q.Epoch,
			)
		}
		requestedEpoch = q.Epoch
	}
	var reqState state.BeaconState
	var err error
	if requestedEpoch != currentEpoch {
		var s primitives.Slot
		s, err = slots.EpochStart(requestedEpoch)
		if err != nil {
			return nil, err
		}
		reqState, err = bs.ReplayerBuilder.ReplayerForSlot(s).ReplayBlocks(ctx)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("error replaying blocks for state at slot %d: %v", s, err))
		}
	} else {
		reqState, err = bs.HeadFetcher.HeadState(ctx)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get requested state: %v", err)
	}
	if reqState == nil || reqState.IsNil() {
		return nil, status.Error(codes.Internal, "Requested state is nil")
	}

	s, err := slots.EpochStart(requestedEpoch)
	if err != nil {
		return nil, err
	}
	if s > reqState.Slot() {
		reqState = reqState.Copy()
		reqState, err = transition.ProcessSlots(ctx, reqState, s)
		if err != nil {
			return nil, status.Errorf(
				codes.Internal,
				"Could not process slots up to epoch %d: %v",
				requestedEpoch,
				err,
			)
		}
	}

	validatorList := make([]*zondpb.Validators_ValidatorContainer, 0)

	for _, index := range req.Indices {
		val, err := reqState.ValidatorAtIndex(index)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not get validator: %v", err)
		}
		validatorList = append(validatorList, &zondpb.Validators_ValidatorContainer{
			Index:     index,
			Validator: val,
		})
	}

	for _, pubKey := range req.PublicKeys {
		// Skip empty public key.
		if len(pubKey) == 0 {
			continue
		}
		pubkeyBytes := bytesutil.ToBytes2592(pubKey)
		index, ok := reqState.ValidatorIndexByPubkey(pubkeyBytes)
		if !ok {
			continue
		}
		val, err := reqState.ValidatorAtIndex(index)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not get validator: %v", err)
		}
		validatorList = append(validatorList, &zondpb.Validators_ValidatorContainer{
			Index:     index,
			Validator: val,
		})
	}
	// Depending on the indices and public keys given, results might not be sorted.
	sort.Slice(validatorList, func(i, j int) bool {
		return validatorList[i].Index < validatorList[j].Index
	})

	if len(req.PublicKeys) == 0 && len(req.Indices) == 0 {
		for i := primitives.ValidatorIndex(0); uint64(i) < uint64(reqState.NumValidators()); i++ {
			val, err := reqState.ValidatorAtIndex(i)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Could not get validator: %v", err)
			}
			validatorList = append(validatorList, &zondpb.Validators_ValidatorContainer{
				Index:     i,
				Validator: val,
			})
		}
	}

	// Filter active validators if the request specifies it.
	res := validatorList
	if req.Active {
		filteredValidators := make([]*zondpb.Validators_ValidatorContainer, 0)
		for _, item := range validatorList {
			if helpers.IsActiveValidator(item.Validator, requestedEpoch) {
				filteredValidators = append(filteredValidators, item)
			}
		}
		res = filteredValidators
	}

	validatorCount := len(res)
	// If there are no items, we simply return a response specifying this.
	// Otherwise, attempting to paginate 0 validators below would result in an error.
	if validatorCount == 0 {
		return &zondpb.Validators{
			ValidatorList: make([]*zondpb.Validators_ValidatorContainer, 0),
			TotalSize:     int32(0),
			NextPageToken: strconv.Itoa(0),
		}, nil
	}

	start, end, nextPageToken, err := pagination.StartAndEndPage(req.PageToken, int(req.PageSize), validatorCount)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"Could not paginate results: %v",
			err,
		)
	}

	return &zondpb.Validators{
		ValidatorList: res[start:end],
		TotalSize:     int32(validatorCount),
		NextPageToken: nextPageToken,
	}, nil
}

// GetValidator information from any validator in the registry by index or public key.
func (bs *Server) GetValidator(
	ctx context.Context, req *zondpb.GetValidatorRequest,
) (*zondpb.Validator, error) {
	var requestingIndex bool
	var index primitives.ValidatorIndex
	var pubKey []byte
	switch q := req.QueryFilter.(type) {
	case *zondpb.GetValidatorRequest_Index:
		index = q.Index
		requestingIndex = true
	case *zondpb.GetValidatorRequest_PublicKey:
		pubKey = q.PublicKey
	default:
		return nil, status.Error(
			codes.InvalidArgument,
			"Need to specify either validator index or public key in request",
		)
	}
	headState, err := bs.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
	}
	if requestingIndex {
		if uint64(index) >= uint64(headState.NumValidators()) {
			return nil, status.Errorf(
				codes.OutOfRange,
				"Requesting index %d, but there are only %d validators",
				index,
				headState.NumValidators(),
			)
		}
		return headState.ValidatorAtIndex(index)
	}
	pk2592 := bytesutil.ToBytes2592(pubKey)
	for i := primitives.ValidatorIndex(0); uint64(i) < uint64(headState.NumValidators()); i++ {
		keyFromState := headState.PubkeyAtIndex(i)
		if keyFromState == pk2592 {
			return headState.ValidatorAtIndex(i)
		}
	}
	return nil, status.Error(codes.NotFound, "No validator matched filter criteria")
}

// GetValidatorActiveSetChanges retrieves the active set changes for a given epoch.
//
// This data includes any activations, voluntary exits, and involuntary
// ejections.
func (bs *Server) GetValidatorActiveSetChanges(
	ctx context.Context, req *zondpb.GetValidatorActiveSetChangesRequest,
) (*zondpb.ActiveSetChanges, error) {
	currentEpoch := slots.ToEpoch(bs.GenesisTimeFetcher.CurrentSlot())

	var requestedEpoch primitives.Epoch
	switch q := req.QueryFilter.(type) {
	case *zondpb.GetValidatorActiveSetChangesRequest_Genesis:
		requestedEpoch = 0
	case *zondpb.GetValidatorActiveSetChangesRequest_Epoch:
		requestedEpoch = q.Epoch
	default:
		requestedEpoch = currentEpoch
	}
	if requestedEpoch > currentEpoch {
		return nil, status.Errorf(
			codes.InvalidArgument,
			errEpoch,
			currentEpoch,
			requestedEpoch,
		)
	}

	s, err := slots.EpochStart(requestedEpoch)
	if err != nil {
		return nil, err
	}
	requestedState, err := bs.ReplayerBuilder.ReplayerForSlot(s).ReplayBlocks(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("error replaying blocks for state at slot %d: %v", s, err))
	}

	activeValidatorCount, err := helpers.ActiveValidatorCount(ctx, requestedState, coreTime.CurrentEpoch(requestedState))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get active validator count: %v", err)
	}
	vs := requestedState.Validators()
	activatedIndices := validators.ActivatedValidatorIndices(coreTime.CurrentEpoch(requestedState), vs)
	exitedIndices, err := validators.ExitedValidatorIndices(coreTime.CurrentEpoch(requestedState), vs, activeValidatorCount)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine exited validator indices: %v", err)
	}
	slashedIndices := validators.SlashedValidatorIndices(coreTime.CurrentEpoch(requestedState), vs)
	ejectedIndices, err := validators.EjectedValidatorIndices(coreTime.CurrentEpoch(requestedState), vs, activeValidatorCount)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine ejected validator indices: %v", err)
	}

	// Retrieve public keys for the indices.
	activatedKeys := make([][]byte, len(activatedIndices))
	exitedKeys := make([][]byte, len(exitedIndices))
	slashedKeys := make([][]byte, len(slashedIndices))
	ejectedKeys := make([][]byte, len(ejectedIndices))
	for i, idx := range activatedIndices {
		pubkey := requestedState.PubkeyAtIndex(idx)
		activatedKeys[i] = pubkey[:]
	}
	for i, idx := range exitedIndices {
		pubkey := requestedState.PubkeyAtIndex(idx)
		exitedKeys[i] = pubkey[:]
	}
	for i, idx := range slashedIndices {
		pubkey := requestedState.PubkeyAtIndex(idx)
		slashedKeys[i] = pubkey[:]
	}
	for i, idx := range ejectedIndices {
		pubkey := requestedState.PubkeyAtIndex(idx)
		ejectedKeys[i] = pubkey[:]
	}
	return &zondpb.ActiveSetChanges{
		Epoch:               requestedEpoch,
		ActivatedPublicKeys: activatedKeys,
		ActivatedIndices:    activatedIndices,
		ExitedPublicKeys:    exitedKeys,
		ExitedIndices:       exitedIndices,
		SlashedPublicKeys:   slashedKeys,
		SlashedIndices:      slashedIndices,
		EjectedPublicKeys:   ejectedKeys,
		EjectedIndices:      ejectedIndices,
	}, nil
}

// GetValidatorParticipation retrieves the validator participation information for a given epoch,
// it returns the information about validator's participation rate in voting on the proof of stake
// rules based on their balance compared to the total active validator balance.
func (bs *Server) GetValidatorParticipation(
	ctx context.Context, req *zondpb.GetValidatorParticipationRequest,
) (*zondpb.ValidatorParticipationResponse, error) {
	currentSlot := bs.GenesisTimeFetcher.CurrentSlot()
	currentEpoch := slots.ToEpoch(currentSlot)

	var requestedEpoch primitives.Epoch
	switch q := req.QueryFilter.(type) {
	case *zondpb.GetValidatorParticipationRequest_Genesis:
		requestedEpoch = 0
	case *zondpb.GetValidatorParticipationRequest_Epoch:
		requestedEpoch = q.Epoch
	default:
		requestedEpoch = currentEpoch
	}

	if requestedEpoch > currentEpoch {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"Cannot retrieve information about an epoch greater than current epoch, current epoch %d, requesting %d",
			currentEpoch,
			requestedEpoch,
		)
	}
	// Use the last slot of requested epoch to obtain current and previous epoch attestations.
	// This ensures that we don't miss previous attestations when input requested epochs.
	endSlot, err := slots.EpochEnd(requestedEpoch)
	if err != nil {
		return nil, err
	}
	// Get as close as we can to the end of the current epoch without going past the current slot.
	// The above check ensures a future *epoch* isn't requested, but the end slot of the requested epoch could still
	// be past the current slot. In that case, use the current slot as the best approximation of the requested epoch.
	// Replayer will make sure the slot ultimately used is canonical.
	if endSlot > currentSlot {
		endSlot = currentSlot
	}

	// ReplayerBuilder ensures that a canonical chain is followed to the slot
	beaconState, err := bs.ReplayerBuilder.ReplayerForSlot(endSlot).ReplayBlocks(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("error replaying blocks for state at slot %d: %v", endSlot, err))
	}
	var v []*precompute.Validator
	var b *precompute.Balance

	v, b, err = altair.InitializePrecomputeValidators(ctx, beaconState)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not set up altair pre compute instance: %v", err)
	}
	_, b, err = altair.ProcessEpochParticipation(ctx, beaconState, b, v)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not pre compute attestations: %v", err)
	}

	cp := bs.FinalizationFetcher.FinalizedCheckpt()
	p := &zondpb.ValidatorParticipationResponse{
		Epoch:     requestedEpoch,
		Finalized: requestedEpoch <= cp.Epoch,
		Participation: &zondpb.ValidatorParticipation{
			CurrentEpochActiveGwei:           b.ActiveCurrentEpoch,
			CurrentEpochAttestingGwei:        b.CurrentEpochAttested,
			CurrentEpochTargetAttestingGwei:  b.CurrentEpochTargetAttested,
			PreviousEpochActiveGwei:          b.ActivePrevEpoch,
			PreviousEpochAttestingGwei:       b.PrevEpochAttested,
			PreviousEpochTargetAttestingGwei: b.PrevEpochTargetAttested,
			PreviousEpochHeadAttestingGwei:   b.PrevEpochHeadAttested,
		},
	}

	return p, nil
}

// GetValidatorPerformance reports the validator's latest balance along with other important metrics on
// rewards and penalties throughout its lifecycle in the beacon chain.
func (bs *Server) GetValidatorPerformance(
	ctx context.Context, req *zondpb.ValidatorPerformanceRequest,
) (*zondpb.ValidatorPerformanceResponse, error) {
	response, err := bs.CoreService.ComputeValidatorPerformance(ctx, req)
	if err != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(err.Reason), "Could not compute validator performance: %v", err.Err)
	}
	return response, nil
}

// GetIndividualVotes retrieves individual voting status of validators.
func (bs *Server) GetIndividualVotes(
	ctx context.Context,
	req *zondpb.IndividualVotesRequest,
) (*zondpb.IndividualVotesRespond, error) {
	currentEpoch := slots.ToEpoch(bs.GenesisTimeFetcher.CurrentSlot())
	if req.Epoch > currentEpoch {
		return nil, status.Errorf(
			codes.InvalidArgument,
			errEpoch,
			currentEpoch,
			req.Epoch,
		)
	}

	s, err := slots.EpochEnd(req.Epoch)
	if err != nil {
		return nil, err
	}
	st, err := bs.ReplayerBuilder.ReplayerForSlot(s).ReplayBlocks(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to replay blocks for state at epoch %d: %v", req.Epoch, err)
	}
	// Track filtered validators to prevent duplication in the response.
	filtered := map[primitives.ValidatorIndex]bool{}
	filteredIndices := make([]primitives.ValidatorIndex, 0)
	votes := make([]*zondpb.IndividualVotesRespond_IndividualVote, 0, len(req.Indices)+len(req.PublicKeys))
	// Filter out assignments by public keys.
	for _, pubKey := range req.PublicKeys {
		index, ok := st.ValidatorIndexByPubkey(bytesutil.ToBytes2592(pubKey))
		if !ok {
			votes = append(votes, &zondpb.IndividualVotesRespond_IndividualVote{PublicKey: pubKey, ValidatorIndex: primitives.ValidatorIndex(^uint64(0))})
			continue
		}
		filtered[index] = true
		filteredIndices = append(filteredIndices, index)
	}
	// Filter out assignments by validator indices.
	for _, index := range req.Indices {
		if !filtered[index] {
			filteredIndices = append(filteredIndices, index)
		}
	}
	sort.Slice(filteredIndices, func(i, j int) bool {
		return filteredIndices[i] < filteredIndices[j]
	})

	var v []*precompute.Validator
	var bal *precompute.Balance
	if st.Version() == version.Capella {
		v, bal, err = altair.InitializePrecomputeValidators(ctx, st)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not set up altair pre compute instance: %v", err)
		}
		v, _, err = altair.ProcessEpochParticipation(ctx, st, bal, v)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not pre compute attestations: %v", err)
		}
	} else {
		return nil, status.Errorf(codes.Internal, "Invalid state type retrieved with a version of %d", st.Version())
	}

	for _, index := range filteredIndices {
		if uint64(index) >= uint64(len(v)) {
			votes = append(votes, &zondpb.IndividualVotesRespond_IndividualVote{ValidatorIndex: index})
			continue
		}
		val, err := st.ValidatorAtIndexReadOnly(index)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not retrieve validator: %v", err)
		}
		pb := val.PublicKey()
		votes = append(votes, &zondpb.IndividualVotesRespond_IndividualVote{
			Epoch:                            req.Epoch,
			PublicKey:                        pb[:],
			ValidatorIndex:                   index,
			IsSlashed:                        v[index].IsSlashed,
			IsWithdrawableInCurrentEpoch:     v[index].IsWithdrawableCurrentEpoch,
			IsActiveInCurrentEpoch:           v[index].IsActiveCurrentEpoch,
			IsActiveInPreviousEpoch:          v[index].IsActivePrevEpoch,
			IsCurrentEpochAttester:           v[index].IsCurrentEpochAttester,
			IsCurrentEpochTargetAttester:     v[index].IsCurrentEpochTargetAttester,
			IsPreviousEpochAttester:          v[index].IsPrevEpochAttester,
			IsPreviousEpochTargetAttester:    v[index].IsPrevEpochTargetAttester,
			IsPreviousEpochHeadAttester:      v[index].IsPrevEpochHeadAttester,
			CurrentEpochEffectiveBalanceGwei: v[index].CurrentEpochEffectiveBalance,
			InactivityScore:                  v[index].InactivityScore,
		})
	}

	return &zondpb.IndividualVotesRespond{
		IndividualVotes: votes,
	}, nil
}

func validatorStatus(validator *zondpb.Validator, epoch primitives.Epoch) zondpb.ValidatorStatus {
	farFutureEpoch := params.BeaconConfig().FarFutureEpoch
	if validator == nil {
		return zondpb.ValidatorStatus_UNKNOWN_STATUS
	}
	if epoch < validator.ActivationEligibilityEpoch {
		return zondpb.ValidatorStatus_DEPOSITED
	}
	if epoch < validator.ActivationEpoch {
		return zondpb.ValidatorStatus_PENDING
	}
	if validator.ExitEpoch == farFutureEpoch {
		return zondpb.ValidatorStatus_ACTIVE
	}
	if epoch < validator.ExitEpoch {
		if validator.Slashed {
			return zondpb.ValidatorStatus_SLASHING
		}
		return zondpb.ValidatorStatus_EXITING
	}
	return zondpb.ValidatorStatus_EXITED
}
