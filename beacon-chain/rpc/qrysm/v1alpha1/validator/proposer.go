package validator

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/blockchain"
	"github.com/theQRL/qrysm/beacon-chain/builder"
	coreblocks "github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	blockfeed "github.com/theQRL/qrysm/beacon-chain/core/feed/block"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/db/kv"
	"github.com/theQRL/qrysm/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
	"go.opencensus.io/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// executionDataNotification is a latch to stop flooding logs with the same warning.
var executionDataNotification bool

const (
	// CouldNotDecodeBlock means that a signed beacon block couldn't be created from the block present in the request.
	CouldNotDecodeBlock  = "Could not decode block"
	executionDataTimeout = 2 * time.Second
)

// GetBeaconBlock is called by a proposer during its assigned slot to request a block to sign
// by passing in the slot and the signed randao reveal of the slot.
func (vs *Server) GetBeaconBlock(ctx context.Context, req *qrysmpb.BlockRequest) (*qrysmpb.GenericBeaconBlock, error) {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.GetBeaconBlock")
	defer span.End()
	span.AddAttributes(trace.Int64Attribute("slot", int64(req.Slot)))

	t, err := slots.ToTime(uint64(vs.TimeFetcher.GenesisTime().Unix()), req.Slot)
	if err != nil {
		log.WithError(err).Error("Could not convert slot to time")
	}
	log.WithFields(logrus.Fields{
		"slot":               req.Slot,
		"sinceSlotStartTime": time.Since(t),
	}).Info("Begin building block")

	// Building a block advances the head state to req.Slot, so bound the work a
	// request can trigger. Two epochs is far beyond any clock skew a proposer
	// could legitimately have.
	if maxSlot := vs.TimeFetcher.CurrentSlot() + 2*params.BeaconConfig().SlotsPerEpoch; req.Slot > maxSlot {
		return nil, status.Errorf(codes.InvalidArgument, "Requested slot %d is too far in the future, latest allowed is %d", req.Slot, maxSlot)
	}

	// A syncing validator should not produce a block.
	if vs.SyncChecker.Syncing() {
		return nil, status.Error(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}

	// An optimistic validator MUST NOT produce a block (i.e., sign across the DOMAIN_BEACON_PROPOSER domain).
	if err := vs.optimisticStatus(ctx); err != nil {
		return nil, status.Errorf(codes.Unavailable, "Validator is not ready to propose: %v", err)
	}

	head, parentRoot, err := vs.getParentState(ctx, req.Slot)
	if err != nil {
		return nil, err
	}
	// Fail fast on a reveal that does not open the proposer's RANDAO
	// commitment: the block would be invalid, so there is no point building
	// it. A zero reveal is allowed for callers that only want a state root
	// (REST skip_randao_verification).
	if len(req.RandaoReveal) != fieldparams.RandaoRevealLength {
		return nil, status.Errorf(codes.InvalidArgument, "randao reveal must be %d bytes, got %d", fieldparams.RandaoRevealLength, len(req.RandaoReveal))
	}
	if reveal := bytesutil.ToBytes32(req.RandaoReveal); reveal != [32]byte{} {
		if err := coreblocks.VerifyRandaoReveal(ctx, head, reveal); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "Could not verify randao reveal: %v", err)
		}
	}
	sBlk, err := getEmptyBlock(req.Slot)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not prepare block: %v", err)
	}

	// Set slot, graffiti, randao reveal, and parent root.
	sBlk.SetSlot(req.Slot)
	sBlk.SetGraffiti(req.Graffiti)
	sBlk.SetRandaoReveal(req.RandaoReveal)
	sBlk.SetParentRoot(parentRoot[:])

	// Set proposer index.
	idx, err := helpers.BeaconProposerIndex(ctx, head)
	if err != nil {
		return nil, fmt.Errorf("could not calculate proposer index %v", err)
	}
	sBlk.SetProposerIndex(idx)

	if err = vs.BuildBlockParallel(ctx, sBlk, head, req.SkipMevBoost); err != nil {
		return nil, errors.Wrap(err, "could not build block in parallel")
	}

	sr, err := vs.computeStateRoot(ctx, sBlk)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not compute state root: %v", err)
	}
	sBlk.SetStateRoot(sr)

	log.WithFields(logrus.Fields{
		"slot":               req.Slot,
		"sinceSlotStartTime": time.Since(t),
		"validator":          sBlk.Block().ProposerIndex(),
	}).Info("Finished building block")

	return vs.constructGenericBeaconBlock(sBlk)
}

func (vs *Server) getParentState(ctx context.Context, slot primitives.Slot) (state.BeaconState, [32]byte, error) {
	// process attestations and update head in forkchoice
	oldHeadRoot := vs.ForkchoiceFetcher.CachedHeadRoot()
	vs.ForkchoiceFetcher.UpdateHead(ctx, vs.TimeFetcher.CurrentSlot())
	headRoot := vs.ForkchoiceFetcher.CachedHeadRoot()
	parentRoot := vs.ForkchoiceFetcher.GetProposerHead()
	head, err := vs.getParentStateFromReorgData(ctx, slot, oldHeadRoot, parentRoot, headRoot)
	return head, parentRoot, err
}

func (vs *Server) getParentStateFromReorgData(ctx context.Context, slot primitives.Slot, oldHeadRoot, parentRoot, headRoot [32]byte) (state.BeaconState, error) {
	var head state.BeaconState
	var err error
	if parentRoot != headRoot {
		head, err = vs.handleSuccessfulReorgAttempt(ctx, slot, parentRoot)
	} else {
		if oldHeadRoot != headRoot {
			logFailedReorgAttempt(slot, oldHeadRoot, headRoot)
		}
		head, err = vs.getHeadNoReorg(ctx, slot, parentRoot)
	}
	if err != nil {
		return nil, err
	}
	if head.Slot() >= slot {
		return head, nil
	}
	head, err = transition.ProcessSlotsUsingNextSlotCache(ctx, head, parentRoot[:], slot)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not process slots up to %d: %v", slot, err)
	}
	return head, nil
}

func (vs *Server) handleSuccessfulReorgAttempt(ctx context.Context, slot primitives.Slot, parentRoot [32]byte) (state.BeaconState, error) {
	// Try to get the state from the NSC
	head := transition.NextSlotState(parentRoot[:], slot)
	if head != nil {
		return head, nil
	}
	// cache miss
	head, err := vs.StateGen.StateByRoot(ctx, parentRoot)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "could not obtain head state")
	}
	return head, nil
}

func logFailedReorgAttempt(slot primitives.Slot, oldHeadRoot, headRoot [32]byte) {
	blockchain.LateBlockAttemptedReorgCount.Inc()
	log.WithFields(logrus.Fields{
		"slot":        slot,
		"oldHeadRoot": fmt.Sprintf("%#x", oldHeadRoot),
		"headRoot":    fmt.Sprintf("%#x", headRoot),
	}).Warn("Late block attempted reorg failed")
}

func (vs *Server) getHeadNoReorg(ctx context.Context, slot primitives.Slot, parentRoot [32]byte) (state.BeaconState, error) {
	// Try to get the state from the NSC
	head := transition.NextSlotState(parentRoot[:], slot)
	if head != nil {
		return head, nil
	}
	// The published head can change after parent selection. Load the state
	// for that selected root, just as the intentional reorg path does.
	head, err := vs.StateGen.StateByRoot(ctx, parentRoot)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get parent state: %v", err)
	}
	return head, nil
}

func (vs *Server) BuildBlockParallel(ctx context.Context, sBlk interfaces.SignedBeaconBlock, head state.BeaconState, skipMevBoost bool) error {
	// Slashing selection applies every pending slashing to the state it selects
	// from, so that exit selection right after it drops exits of validators
	// slashed in this block. Give the consensus-fields goroutine its own copy:
	// the execution payload's withdrawals are derived from the state handed to
	// getLocalPayload, and the transition validates them against the state
	// before this block's operations, so that state must never see those
	// mutations while the two run concurrently.
	consensusHead := head.Copy()

	// Build consensus fields in background
	var wg sync.WaitGroup
	wg.Go(func() {

		// Set execution data.
		executionData, err := vs.executionDataMajorityVote(ctx, consensusHead)
		if err != nil {
			executionData = &qrysmpb.ExecutionData{DepositRoot: params.BeaconConfig().ZeroHash[:], BlockHash: params.BeaconConfig().ZeroHash[:]}
			log.WithError(err).Error("Could not get executiondata")
		}
		sBlk.SetExecutionData(executionData)

		// Set deposit and attestation.
		deposits, atts, err := vs.packDepositsAndAttestations(ctx, consensusHead, executionData) // TODO: split attestations and deposits
		if err != nil {
			sBlk.SetDeposits([]*qrysmpb.Deposit{})
			sBlk.SetAttestations([]*qrysmpb.Attestation{})
			log.WithError(err).Error("Could not pack deposits and attestations")
		} else {
			sBlk.SetDeposits(deposits)
			sBlk.SetAttestations(atts)
		}

		// Set slashings.
		validProposerSlashings, validAttSlashings := vs.getSlashings(ctx, consensusHead)
		sBlk.SetProposerSlashings(validProposerSlashings)
		sBlk.SetAttesterSlashings(validAttSlashings)

		// Set exits.
		sBlk.SetVoluntaryExits(vs.getExits(consensusHead, sBlk.Block().Slot()))

		// Set sync aggregate.
		vs.setSyncAggregate(ctx, sBlk, consensusHead)
	})

	localPayload, overrideBuilder, err := vs.getLocalPayload(ctx, sBlk.Block(), head)
	if err != nil {
		return status.Errorf(codes.Internal, "Could not get local payload: %v", err)
	}

	// There's no reason to try to get a builder bid if local override is true.
	var builderPayload interfaces.ExecutionData
	overrideBuilder = overrideBuilder || skipMevBoost // Skip using mev-boost if requested by the caller.
	if !overrideBuilder {
		latestHeader, headerErr := head.LatestExecutionPayloadHeader()
		if headerErr != nil {
			return status.Errorf(codes.Internal, "Could not get latest execution payload header: %v", headerErr)
		}
		parentGasLimit := latestHeader.GasLimit()
		builderPayload, err = vs.getBuilderPayload(ctx, sBlk.Block().Slot(), sBlk.Block().ProposerIndex(), parentGasLimit)
		if err != nil {
			builderGetPayloadMissCount.Inc()
			log.WithError(err).Error("Could not get builder payload")
		}
	}

	if err := setExecutionData(ctx, sBlk, localPayload, builderPayload); err != nil {
		return status.Errorf(codes.Internal, "Could not set execution data: %v", err)
	}

	wg.Wait() // Wait until block is built via consensus and execution fields.

	return nil
}

// ProposeBeaconBlock is called by a proposer during its assigned slot to create a block in an attempt
// to get it processed by the beacon node as the canonical head.
func (vs *Server) ProposeBeaconBlock(ctx context.Context, req *qrysmpb.GenericSignedBeaconBlock) (*qrysmpb.ProposeResponse, error) {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.ProposeBeaconBlock")
	defer span.End()

	if err := validateProposedBlockFieldLengths(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s: %v", CouldNotDecodeBlock, err)
	}

	blk, err := blocks.NewSignedBeaconBlock(req.Block)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s: %v", CouldNotDecodeBlock, err)
	}

	unblinder, err := newUnblinder(blk, vs.BlockBuilder)
	if err != nil {
		return nil, errors.Wrap(err, "could not create unblinder")
	}

	blk, err = unblinder.unblindBuilderBlock(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not unblind builder block")
	}

	// Broadcast the new block to the network.
	blkPb, err := blk.Proto()
	if err != nil {
		return nil, errors.Wrap(err, "could not get protobuf block")
	}
	if err := vs.P2P.Broadcast(ctx, blkPb); err != nil {
		return nil, fmt.Errorf("could not broadcast block: %v", err)
	}

	root, err := blk.Block().HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("could not tree hash block: %v", err)
	}
	log.WithFields(logrus.Fields{
		"blockRoot": hex.EncodeToString(root[:]),
	}).Debug("Broadcasting block")

	if err := vs.BlockReceiver.ReceiveBlock(ctx, blk, root); err != nil {
		if !blockchain.IsUnrelatedBlockError(err) {
			return nil, fmt.Errorf("could not process beacon block: %v", err)
		}
		// The execution rejection concerns another branch. This block was
		// imported, so the proposal succeeded.
		log.WithError(err).WithField("slot", blk.Block().Slot()).Warn("Proposed block imported while another branch was invalidated")
	}

	log.WithField("slot", blk.Block().Slot()).Debugf(
		"Block proposal received via RPC")
	vs.BlockNotifier.BlockFeed().Send(&feed.Event{
		Type: blockfeed.ReceivedBlock,
		Data: &blockfeed.ReceivedBlockData{SignedBlock: blk},
	})

	return &qrysmpb.ProposeResponse{
		BlockRoot: root[:],
	}, nil
}

// validateProposedBlockFieldLengths checks fields whose lengths would be lost
// when NewSignedBeaconBlock copies them into fixed-size arrays. Validate at the
// RPC boundary so internal callers can still construct unfinished blocks.
func validateProposedBlockFieldLengths(req *qrysmpb.GenericSignedBeaconBlock) error {
	var signature, parentRoot, stateRoot, randaoReveal, graffiti []byte
	switch b := req.GetBlock().(type) {
	case nil:
		return blocks.ErrNilObject
	case *qrysmpb.GenericSignedBeaconBlock_Zond:
		if b == nil {
			return blocks.ErrNilObject
		}
		block := b.Zond.GetBlock()
		body := block.GetBody()
		signature = b.Zond.GetSignature()
		parentRoot, stateRoot = block.GetParentRoot(), block.GetStateRoot()
		randaoReveal, graffiti = body.GetRandaoReveal(), body.GetGraffiti()
	case *qrysmpb.GenericSignedBeaconBlock_BlindedZond:
		if b == nil {
			return blocks.ErrNilObject
		}
		block := b.BlindedZond.GetBlock()
		body := block.GetBody()
		signature = b.BlindedZond.GetSignature()
		parentRoot, stateRoot = block.GetParentRoot(), block.GetStateRoot()
		randaoReveal, graffiti = body.GetRandaoReveal(), body.GetGraffiti()
	default:
		return errors.Wrapf(blocks.ErrUnsupportedSignedBeaconBlock, "unable to validate block from type %T", b)
	}
	for _, field := range []struct {
		name  string
		value []byte
		size  int
	}{
		{"signature", signature, fieldparams.MLDSA87SignatureLength},
		{"parent root", parentRoot, fieldparams.RootLength},
		{"state root", stateRoot, fieldparams.RootLength},
		{"randao reveal", randaoReveal, fieldparams.RandaoRevealLength},
		{"graffiti", graffiti, fieldparams.RootLength},
	} {
		if len(field.value) != field.size {
			return fmt.Errorf("%s must be %d bytes, got %d", field.name, field.size, len(field.value))
		}
	}
	return nil
}

// PrepareBeaconProposer caches and updates the fee recipient for the given proposer.
func (vs *Server) PrepareBeaconProposer(
	ctx context.Context, request *qrysmpb.PrepareBeaconProposerRequest,
) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "validator.PrepareBeaconProposer")
	defer span.End()
	var feeRecipients []common.Address
	var validatorIndices []primitives.ValidatorIndex

	// Fee recipients are stored by validator index and never removed, so only
	// indices of validators in the registry are accepted; otherwise any API
	// client could grow the database without bound.
	headState, err := vs.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
	}
	numValidators := uint64(headState.NumValidators())
	newRecipients := make([]*qrysmpb.PrepareBeaconProposerRequest_FeeRecipientContainer, 0, len(request.Recipients))
	for _, r := range request.Recipients {
		if uint64(r.ValidatorIndex) >= numValidators {
			return nil, status.Errorf(codes.InvalidArgument, "Validator index %d is not in the registry of %d validators", r.ValidatorIndex, numValidators)
		}
		f, err := vs.BeaconDB.FeeRecipientByValidatorID(ctx, r.ValidatorIndex)
		switch {
		case errors.Is(err, kv.ErrNotFoundFeeRecipient):
			newRecipients = append(newRecipients, r)
		case err != nil:
			return nil, status.Errorf(codes.Internal, "Could not get fee recipient by validator index: %v", err)
		default:
			if common.BytesToAddress(r.FeeRecipient) != f {
				newRecipients = append(newRecipients, r)
			}
		}
	}
	if len(newRecipients) == 0 {
		return &emptypb.Empty{}, nil
	}

	for _, recipientContainer := range newRecipients {
		recipient := hexutil.EncodeQ(recipientContainer.FeeRecipient)
		if !common.IsAddress(recipient) {
			return nil, status.Errorf(codes.InvalidArgument, "Invalid fee recipient address: %v", recipient)
		}
		feeRecipients = append(feeRecipients, common.BytesToAddress(recipientContainer.FeeRecipient))
		validatorIndices = append(validatorIndices, recipientContainer.ValidatorIndex)
	}
	if err := vs.BeaconDB.SaveFeeRecipientsByValidatorIDs(ctx, validatorIndices, feeRecipients); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not save fee recipients: %v", err)
	}
	log.WithFields(logrus.Fields{
		"validatorIndices": validatorIndices,
	}).Info("Updated fee recipient addresses for validator indices")
	return &emptypb.Empty{}, nil
}

// GetFeeRecipientByPubKey returns a fee recipient from the beacon node's settings or db based on a given public key
func (vs *Server) GetFeeRecipientByPubKey(ctx context.Context, request *qrysmpb.FeeRecipientByPubKeyRequest) (*qrysmpb.FeeRecipientByPubKeyResponse, error) {
	ctx, span := trace.StartSpan(ctx, "validator.GetFeeRecipientByPublicKey")
	defer span.End()
	if request == nil {
		return nil, status.Errorf(codes.InvalidArgument, "request was empty")
	}

	resp, err := vs.ValidatorIndex(ctx, &qrysmpb.ValidatorIndexRequest{PublicKey: request.PublicKey})
	if err != nil {
		if strings.Contains(err.Error(), "Could not find validator index") {
			return &qrysmpb.FeeRecipientByPubKeyResponse{
				FeeRecipient: params.BeaconConfig().DefaultFeeRecipient.Bytes(),
			}, nil
		} else {
			log.WithError(err).Error("An error occurred while retrieving validator index")
			return nil, err
		}
	}
	address, err := vs.BeaconDB.FeeRecipientByValidatorID(ctx, resp.GetIndex())
	if err != nil {
		if errors.Is(err, kv.ErrNotFoundFeeRecipient) {
			return &qrysmpb.FeeRecipientByPubKeyResponse{
				FeeRecipient: params.BeaconConfig().DefaultFeeRecipient.Bytes(),
			}, nil
		} else {
			log.WithError(err).Error("An error occurred while retrieving fee recipient from db")
			return nil, status.Errorf(codes.Internal, "error=%s", err)
		}
	}
	return &qrysmpb.FeeRecipientByPubKeyResponse{
		FeeRecipient: address.Bytes(),
	}, nil
}

// computeStateRoot computes the state root after a block has been processed through a state transition and
// returns it to the validator client.
func (vs *Server) computeStateRoot(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock) ([]byte, error) {
	beaconState, err := vs.StateGen.StateByRoot(ctx, block.Block().ParentRoot())
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve beacon state")
	}
	root, err := transition.CalculateStateRoot(
		ctx,
		beaconState,
		block,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "could not calculate state root at slot %d", beaconState.Slot())
	}

	log.WithField("beaconStateRoot", fmt.Sprintf("%#x", root)).Debugf("Computed state root")
	return root[:], nil
}

// SubmitValidatorRegistrations submits validator registrations.
func (vs *Server) SubmitValidatorRegistrations(ctx context.Context, reg *qrysmpb.SignedValidatorRegistrationsV1) (*emptypb.Empty, error) {
	if vs.BlockBuilder == nil || !vs.BlockBuilder.Configured() {
		return &emptypb.Empty{}, status.Errorf(codes.InvalidArgument, "Could not register block builder: %v", builder.ErrNoBuilder)
	}

	if err := vs.BlockBuilder.RegisterValidator(ctx, reg.Messages); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Could not register block builder: %v", err)
	}

	return &emptypb.Empty{}, nil
}
