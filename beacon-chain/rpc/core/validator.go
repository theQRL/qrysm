package core

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/epoch/precompute"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	opfeed "github.com/theQRL/qrysm/beacon-chain/core/feed/operation"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	coreTime "github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/consensus-types/validator"
	"github.com/theQRL/qrysm/crypto/rand"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation"
	qrysmTime "github.com/theQRL/qrysm/time"
	"github.com/theQRL/qrysm/time/slots"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
)

// AggregateBroadcastFailedError represents an error scenario where
// broadcasting an aggregate selection proof failed.
type AggregateBroadcastFailedError struct {
	err error
}

// NewAggregateBroadcastFailedError creates a new error instance.
func NewAggregateBroadcastFailedError(err error) AggregateBroadcastFailedError {
	return AggregateBroadcastFailedError{
		err: err,
	}
}

// Error returns the underlying error message.
func (e *AggregateBroadcastFailedError) Error() string {
	return fmt.Sprintf("could not broadcast signed aggregated attestation: %s", e.err.Error())
}

// ComputeValidatorPerformance reports the validator's latest balance along with other important metrics on
// rewards and penalties throughout its lifecycle in the beacon chain.
func (s *Service) ComputeValidatorPerformance(
	ctx context.Context,
	req *zondpb.ValidatorPerformanceRequest,
) (*zondpb.ValidatorPerformanceResponse, *RpcError) {
	if s.SyncChecker.Syncing() {
		return nil, &RpcError{Reason: Unavailable, Err: errors.New("Syncing to latest head, not ready to respond")}
	}

	headState, err := s.HeadFetcher.HeadState(ctx)
	if err != nil {
		return nil, &RpcError{Err: errors.Wrap(err, "could not get head state"), Reason: Internal}
	}
	currSlot := s.GenesisTimeFetcher.CurrentSlot()
	if currSlot > headState.Slot() {
		headRoot, err := s.HeadFetcher.HeadRoot(ctx)
		if err != nil {
			return nil, &RpcError{Err: errors.Wrap(err, "could not get head root"), Reason: Internal}
		}
		headState, err = transition.ProcessSlotsUsingNextSlotCache(ctx, headState, headRoot, currSlot)
		if err != nil {
			return nil, &RpcError{Err: errors.Wrapf(err, "could not process slots up to %d", currSlot), Reason: Internal}
		}
	}
	var validatorSummary []*precompute.Validator
	vp, bp, err := altair.InitializePrecomputeValidators(ctx, headState)
	if err != nil {
		return nil, &RpcError{Err: err, Reason: Internal}
	}
	vp, bp, err = altair.ProcessEpochParticipation(ctx, headState, bp, vp)
	if err != nil {
		return nil, &RpcError{Err: err, Reason: Internal}
	}
	headState, vp, err = altair.ProcessInactivityScores(ctx, headState, vp)
	if err != nil {
		return nil, &RpcError{Err: err, Reason: Internal}
	}
	headState, err = altair.ProcessRewardsAndPenaltiesPrecompute(headState, bp, vp)
	if err != nil {
		return nil, &RpcError{Err: err, Reason: Internal}
	}
	validatorSummary = vp

	responseCap := len(req.Indices) + len(req.PublicKeys)
	validatorIndices := make([]primitives.ValidatorIndex, 0, responseCap)
	missingValidators := make([][]byte, 0, responseCap)

	filtered := map[primitives.ValidatorIndex]bool{} // Track filtered validators to prevent duplication in the response.
	// Convert the list of validator public keys to validator indices and add to the indices set.
	for _, pubKey := range req.PublicKeys {
		// Skip empty public key.
		if len(pubKey) == 0 {
			continue
		}
		pubkeyBytes := bytesutil.ToBytes2592(pubKey)
		idx, ok := headState.ValidatorIndexByPubkey(pubkeyBytes)
		if !ok {
			// Validator index not found, track as missing.
			missingValidators = append(missingValidators, pubKey)
			continue
		}
		if !filtered[idx] {
			validatorIndices = append(validatorIndices, idx)
			filtered[idx] = true
		}
	}
	// Add provided indices to the indices set.
	for _, idx := range req.Indices {
		if !filtered[idx] {
			validatorIndices = append(validatorIndices, idx)
			filtered[idx] = true
		}
	}
	// Depending on the indices and public keys given, results might not be sorted.
	sort.Slice(validatorIndices, func(i, j int) bool {
		return validatorIndices[i] < validatorIndices[j]
	})

	currentEpoch := coreTime.CurrentEpoch(headState)
	responseCap = len(validatorIndices)
	pubKeys := make([][]byte, 0, responseCap)
	beforeTransitionBalances := make([]uint64, 0, responseCap)
	afterTransitionBalances := make([]uint64, 0, responseCap)
	effectiveBalances := make([]uint64, 0, responseCap)
	correctlyVotedSource := make([]bool, 0, responseCap)
	correctlyVotedTarget := make([]bool, 0, responseCap)
	correctlyVotedHead := make([]bool, 0, responseCap)
	inactivityScores := make([]uint64, 0, responseCap)
	// Append performance summaries.
	// Also track missing validators using public keys.
	for _, idx := range validatorIndices {
		val, err := headState.ValidatorAtIndexReadOnly(idx)
		if err != nil {
			return nil, &RpcError{Err: errors.Wrap(err, "could not get validator"), Reason: Internal}
		}
		pubKey := val.PublicKey()
		if uint64(idx) >= uint64(len(validatorSummary)) {
			// Not listed in validator summary yet; treat it as missing.
			missingValidators = append(missingValidators, pubKey[:])
			continue
		}
		if !helpers.IsActiveValidatorUsingTrie(val, currentEpoch) {
			// Inactive validator; treat it as missing.
			missingValidators = append(missingValidators, pubKey[:])
			continue
		}

		summary := validatorSummary[idx]
		pubKeys = append(pubKeys, pubKey[:])
		effectiveBalances = append(effectiveBalances, summary.CurrentEpochEffectiveBalance)
		beforeTransitionBalances = append(beforeTransitionBalances, summary.BeforeEpochTransitionBalance)
		afterTransitionBalances = append(afterTransitionBalances, summary.AfterEpochTransitionBalance)
		correctlyVotedTarget = append(correctlyVotedTarget, summary.IsPrevEpochTargetAttester)
		correctlyVotedHead = append(correctlyVotedHead, summary.IsPrevEpochHeadAttester)

		correctlyVotedSource = append(correctlyVotedSource, summary.IsPrevEpochSourceAttester)
		inactivityScores = append(inactivityScores, summary.InactivityScore)
	}

	return &zondpb.ValidatorPerformanceResponse{
		PublicKeys:                    pubKeys,
		CorrectlyVotedSource:          correctlyVotedSource,
		CorrectlyVotedTarget:          correctlyVotedTarget, // In altair, when this is true then the attestation was definitely included.
		CorrectlyVotedHead:            correctlyVotedHead,
		CurrentEffectiveBalances:      effectiveBalances,
		BalancesBeforeEpochTransition: beforeTransitionBalances,
		BalancesAfterEpochTransition:  afterTransitionBalances,
		MissingValidators:             missingValidators,
		InactivityScores:              inactivityScores, // Only populated in Altair
	}, nil
}

// SubmitSignedContributionAndProof is called by a sync committee aggregator
// to submit signed contribution and proof object.
func (s *Service) SubmitSignedContributionAndProof(
	ctx context.Context,
	req *zondpb.SignedContributionAndProof,
) *RpcError {
	errs, ctx := errgroup.WithContext(ctx)

	// Broadcasting and saving contribution into the pool in parallel. As one fail should not affect another.
	errs.Go(func() error {
		return s.Broadcaster.Broadcast(ctx, req)
	})

	if err := s.SyncCommitteePool.SaveSyncCommitteeContribution(req.Message.Contribution); err != nil {
		return &RpcError{Err: err, Reason: Internal}
	}

	// Wait for p2p broadcast to complete and return the first error (if any)
	err := errs.Wait()
	if err != nil {
		return &RpcError{Err: err, Reason: Internal}
	}

	s.OperationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.SyncCommitteeContributionReceived,
		Data: &opfeed.SyncCommitteeContributionReceivedData{
			Contribution: req,
		},
	})

	return nil
}

// SubmitSignedAggregateSelectionProof verifies given aggregate and proofs and publishes them on appropriate gossipsub topic.
func (s *Service) SubmitSignedAggregateSelectionProof(
	ctx context.Context,
	req *zondpb.SignedAggregateSubmitRequest,
) *RpcError {
	if req.SignedAggregateAndProof == nil || req.SignedAggregateAndProof.Message == nil ||
		req.SignedAggregateAndProof.Message.Aggregate == nil || req.SignedAggregateAndProof.Message.Aggregate.Data == nil {
		return &RpcError{Err: errors.New("signed aggregate request can't be nil"), Reason: BadRequest}
	}
	emptySig := make([]byte, field_params.DilithiumSignatureLength)
	if bytes.Equal(req.SignedAggregateAndProof.Signature, emptySig) ||
		bytes.Equal(req.SignedAggregateAndProof.Message.SelectionProof, emptySig) {
		return &RpcError{Err: errors.New("signed signatures can't be zero hashes"), Reason: BadRequest}
	}

	// As a preventive measure, a beacon node shouldn't broadcast an attestation whose slot is out of range.
	if err := helpers.ValidateAttestationTime(req.SignedAggregateAndProof.Message.Aggregate.Data.Slot,
		s.GenesisTimeFetcher.GenesisTime(), params.BeaconNetworkConfig().MaximumGossipClockDisparity); err != nil {
		return &RpcError{Err: errors.New("attestation slot is no longer valid from current time"), Reason: BadRequest}
	}

	if err := s.Broadcaster.Broadcast(ctx, req.SignedAggregateAndProof); err != nil {
		return &RpcError{Err: &AggregateBroadcastFailedError{err: err}, Reason: Internal}
	}

	log.WithFields(logrus.Fields{
		"slot":            req.SignedAggregateAndProof.Message.Aggregate.Data.Slot,
		"committeeIndex":  req.SignedAggregateAndProof.Message.Aggregate.Data.CommitteeIndex,
		"validatorIndex":  req.SignedAggregateAndProof.Message.AggregatorIndex,
		"aggregatedCount": req.SignedAggregateAndProof.Message.Aggregate.AggregationBits.Count(),
	}).Debug("Broadcasting aggregated attestation and proof")

	return nil
}

// SignaturesAndAggregationBits returns the signatures and aggregation bits
// associated with a particular set of sync committee messages.
func (s *Service) SignaturesAndAggregationBits(
	ctx context.Context,
	req *zondpb.SignaturesAndAggregationBitsRequest) ([][]byte, []byte, error) {
	subCommitteeSize := params.BeaconConfig().SyncCommitteeSize / params.BeaconConfig().SyncCommitteeSubnetCount
	sigs := make([][]byte, 0, subCommitteeSize)
	bits := zondpb.NewSyncCommitteeAggregationBits()

	for _, msg := range req.Msgs {
		if bytes.Equal(req.BlockRoot, msg.BlockRoot) {
			headSyncCommitteeIndices, err := s.HeadFetcher.HeadSyncCommitteeIndices(ctx, msg.ValidatorIndex, req.Slot)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "could not get sync subcommittee index")
			}
			for _, index := range headSyncCommitteeIndices {
				i := uint64(index)
				subnetIndex := i / subCommitteeSize
				indexMod := i % subCommitteeSize
				if subnetIndex == req.SubnetId && !bits.BitAt(indexMod) {
					insertIdx, err := attestation.SearchInsertIdxWithOffset(bits.BitIndices(), 0, int(indexMod))
					if err != nil {
						return nil, nil, errors.Wrapf(err, "could not get signature insert index")
					}
					bits.SetBitAt(indexMod, true)
					sigs = slices.Insert(sigs, insertIdx, msg.Signature)
				}
			}
		}
	}

	return sigs, bits, nil
}

// AssignValidatorToSubnet checks the status and pubkey of a particular validator
// to discern whether persistent subnets need to be registered for them.
func AssignValidatorToSubnet(pubkey []byte, status validator.ValidatorStatus) {
	if status != validator.Active {
		return
	}
	assignValidatorToSubnet(pubkey)
}

// AssignValidatorToSubnetProto checks the status and pubkey of a particular validator
// to discern whether persistent subnets need to be registered for them.
//
// It has a Proto suffix because the status is a protobuf type.
func AssignValidatorToSubnetProto(pubkey []byte, status zondpb.ValidatorStatus) {
	if status != zondpb.ValidatorStatus_ACTIVE && status != zondpb.ValidatorStatus_EXITING {
		return
	}
	assignValidatorToSubnet(pubkey)
}

func assignValidatorToSubnet(pubkey []byte) {
	_, ok, expTime := cache.SubnetIDs.GetPersistentSubnets(pubkey)
	if ok && expTime.After(qrysmTime.Now()) {
		return
	}
	epochDuration := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	var assignedIdxs []uint64
	randGen := rand.NewGenerator()
	for i := uint64(0); i < params.BeaconConfig().RandomSubnetsPerValidator; i++ {
		assignedIdx := randGen.Intn(int(params.BeaconNetworkConfig().AttestationSubnetCount))
		assignedIdxs = append(assignedIdxs, uint64(assignedIdx))
	}

	assignedDuration := uint64(randGen.Intn(int(params.BeaconConfig().EpochsPerRandomSubnetSubscription)))
	assignedDuration += params.BeaconConfig().EpochsPerRandomSubnetSubscription

	totalDuration := epochDuration * time.Duration(assignedDuration)
	cache.SubnetIDs.AddPersistentCommittee(pubkey, assignedIdxs, totalDuration*time.Second)
}

// GetAttestationData requests that the beacon node produces attestation data for
// the requested committee index and slot based on the nodes current head.
func (s *Service) GetAttestationData(
	ctx context.Context, req *zondpb.AttestationDataRequest,
) (*zondpb.AttestationData, *RpcError) {
	if err := helpers.ValidateAttestationTime(
		req.Slot,
		s.GenesisTimeFetcher.GenesisTime(),
		params.BeaconNetworkConfig().MaximumGossipClockDisparity,
	); err != nil {
		return nil, &RpcError{Reason: BadRequest, Err: errors.Errorf("invalid request: %v", err)}
	}

	res, err := s.AttestationCache.Get(ctx, req)
	if err != nil {
		return nil, &RpcError{Reason: Internal, Err: errors.Errorf("could not retrieve data from attestation cache: %v", err)}
	}
	if res != nil {
		res.CommitteeIndex = req.CommitteeIndex
		return res, nil
	}

	if err := s.AttestationCache.MarkInProgress(req); err != nil {
		if errors.Is(err, cache.ErrAlreadyInProgress) {
			res, err := s.AttestationCache.Get(ctx, req)
			if err != nil {
				return nil, &RpcError{Reason: Internal, Err: errors.Errorf("could not retrieve data from attestation cache: %v", err)}
			}
			if res == nil {
				return nil, &RpcError{Reason: Internal, Err: errors.New("a request was in progress and resolved to nil")}
			}
			res.CommitteeIndex = req.CommitteeIndex
			return res, nil
		}
		return nil, &RpcError{Reason: Internal, Err: errors.Errorf("could not mark attestation as in-progress: %v", err)}
	}
	defer func() {
		if err := s.AttestationCache.MarkNotInProgress(req); err != nil {
			log.WithError(err).Error("could not mark attestation as not-in-progress")
		}
	}()

	headState, err := s.HeadFetcher.HeadState(ctx)
	if err != nil {
		return nil, &RpcError{Reason: Internal, Err: errors.Errorf("could not retrieve head state: %v", err)}
	}
	headRoot, err := s.HeadFetcher.HeadRoot(ctx)
	if err != nil {
		return nil, &RpcError{Reason: Internal, Err: errors.Errorf("could not retrieve head root: %v", err)}
	}

	// In the case that we receive an attestation request after a newer state/block has been processed.
	if headState.Slot() > req.Slot {
		headRoot, err = helpers.BlockRootAtSlot(headState, req.Slot)
		if err != nil {
			return nil, &RpcError{Reason: Internal, Err: errors.Errorf("could not get historical head root: %v", err)}
		}
		headState, err = s.StateGen.StateByRoot(ctx, bytesutil.ToBytes32(headRoot))
		if err != nil {
			return nil, &RpcError{Reason: Internal, Err: errors.Errorf("could not get historical head state: %v", err)}
		}
	}
	if headState == nil || headState.IsNil() {
		return nil, &RpcError{Reason: Internal, Err: errors.New("could not lookup parent state from head")}
	}

	if coreTime.CurrentEpoch(headState) < slots.ToEpoch(req.Slot) {
		headState, err = transition.ProcessSlotsUsingNextSlotCache(ctx, headState, headRoot, req.Slot)
		if err != nil {
			return nil, &RpcError{Reason: Internal, Err: errors.Errorf("could not process slots up to %d: %v", req.Slot, err)}
		}
	}

	targetEpoch := coreTime.CurrentEpoch(headState)
	epochStartSlot, err := slots.EpochStart(targetEpoch)
	if err != nil {
		return nil, &RpcError{Reason: Internal, Err: errors.Errorf("could not calculate epoch start: %v", err)}
	}
	var targetRoot []byte
	if epochStartSlot == headState.Slot() {
		targetRoot = headRoot
	} else {
		targetRoot, err = helpers.BlockRootAtSlot(headState, epochStartSlot)
		if err != nil {
			return nil, &RpcError{Reason: Internal, Err: errors.Errorf("could not get target block for slot %d: %v", epochStartSlot, err)}
		}
		if bytesutil.ToBytes32(targetRoot) == params.BeaconConfig().ZeroHash {
			targetRoot = headRoot
		}
	}

	res = &zondpb.AttestationData{
		Slot:            req.Slot,
		CommitteeIndex:  req.CommitteeIndex,
		BeaconBlockRoot: headRoot,
		Source:          headState.CurrentJustifiedCheckpoint(),
		Target: &zondpb.Checkpoint{
			Epoch: targetEpoch,
			Root:  targetRoot,
		},
	}

	if err := s.AttestationCache.Put(ctx, req, res); err != nil {
		log.WithError(err).Error("could not store attestation data in cache")
	}
	return res, nil
}

// SubmitSyncMessage submits the sync committee message to the network.
// It also saves the sync committee message into the pending pool for block inclusion.
func (s *Service) SubmitSyncMessage(ctx context.Context, msg *zondpb.SyncCommitteeMessage) *RpcError {
	errs, ctx := errgroup.WithContext(ctx)

	headSyncCommitteeIndices, err := s.HeadFetcher.HeadSyncCommitteeIndices(ctx, msg.ValidatorIndex, msg.Slot)
	if err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not get head sync committee indices")}
	}
	// Broadcasting and saving message into the pool in parallel. As one fail should not affect another.
	// This broadcasts for all subnets.
	for _, index := range headSyncCommitteeIndices {
		subCommitteeSize := params.BeaconConfig().SyncCommitteeSize / params.BeaconConfig().SyncCommitteeSubnetCount
		subnet := uint64(index) / subCommitteeSize
		errs.Go(func() error {
			return s.P2P.BroadcastSyncCommitteeMessage(ctx, subnet, msg)
		})
	}

	if err := s.SyncCommitteePool.SaveSyncCommitteeMessage(msg); err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not save sync committee message")}
	}

	// Wait for p2p broadcast to complete and return the first error (if any)
	if err = errs.Wait(); err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not broadcast sync committee message")}
	}
	return nil
}
