package evaluators

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/pkg/errors"
	corehelpers "github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/encoding/ssz/detect"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/endtoend/helpers"
	e2e "github.com/theQRL/qrysm/testing/endtoend/params"
	"github.com/theQRL/qrysm/testing/endtoend/policies"
	e2etypes "github.com/theQRL/qrysm/testing/endtoend/types"
	"github.com/theQRL/qrysm/testing/util"
	"golang.org/x/exp/rand"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

var depositValCount = e2e.DepositCount
var numOfExits = 2

var followDistanceSeconds = params.E2ETestConfig().ExecutionFollowDistance * params.E2ETestConfig().SecondsPerExecutionBlock
var secondsPerEpoch = params.E2ETestConfig().SecondsPerSlot * uint64(params.E2ETestConfig().SlotsPerEpoch)

// Deposits should be processed in twice the length of the epochs per execution voting period.
var depositsInBlockStart = primitives.Epoch(2*followDistanceSeconds/secondsPerEpoch) + params.E2ETestConfig().EpochsPerExecutionVotingPeriod*2

// deposits included + finalization + MaxSeedLookahead for activation.
var depositActivationStartEpoch = depositsInBlockStart + 2 + params.E2ETestConfig().MaxSeedLookahead
var depositEndEpoch = depositActivationStartEpoch + primitives.Epoch(math.Ceil(float64(depositValCount)/float64(params.E2ETestConfig().MinPerEpochChurnLimit)))
var exitSubmissionEpoch = primitives.Epoch(7)

// ProcessesDepositsInBlocks ensures the expected amount of deposits are accepted into blocks.
var ProcessesDepositsInBlocks = e2etypes.Evaluator{
	Name:       "processes_deposits_in_blocks_epoch_%d",
	Policy:     policies.OnEpoch(depositsInBlockStart), // We expect all deposits to enter in one epoch.
	Evaluation: processesDepositsInBlocks,
}

// VerifyBlockGraffiti ensures the block graffiti is one of the random list.
var VerifyBlockGraffiti = e2etypes.Evaluator{
	Name:       "verify_graffiti_in_blocks_epoch_%d",
	Policy:     policies.AfterNthEpoch(0),
	Evaluation: verifyGraffitiInBlocks,
}

// ActivatesDepositedValidators ensures the expected amount of validator deposits are activated into the state.
var ActivatesDepositedValidators = e2etypes.Evaluator{
	Name:       "processes_deposit_validators_epoch_%d",
	Policy:     policies.BetweenEpochs(depositActivationStartEpoch, depositEndEpoch),
	Evaluation: activatesDepositedValidators,
}

// DepositedValidatorsAreActive ensures the expected amount of validators are active after their deposits are processed.
var DepositedValidatorsAreActive = e2etypes.Evaluator{
	Name:       "deposited_validators_are_active_epoch_%d",
	Policy:     policies.AfterNthEpoch(depositEndEpoch),
	Evaluation: depositedValidatorsAreActive,
}

// ProposeVoluntaryExit sends a voluntary exit from randomly selected validator in the genesis set.
var ProposeVoluntaryExit = e2etypes.Evaluator{
	Name:       "propose_voluntary_exit_epoch_%d",
	Policy:     policies.OnEpoch(exitSubmissionEpoch),
	Evaluation: proposeVoluntaryExit,
}

// ValidatorsHaveExited checks the beacon state for the exited validator and ensures its marked as exited.
var ValidatorsHaveExited = e2etypes.Evaluator{
	Name:       "voluntary_has_exited_%d",
	Policy:     policies.OnEpoch(8),
	Evaluation: validatorsHaveExited,
}

// ValidatorsHaveWithdrawn checks the beacon state for the withdrawn validator and ensures it has been withdrawn.
var ValidatorsHaveWithdrawn = e2etypes.Evaluator{
	Name: "validator_has_withdrawn_%d",
	Policy: func(currentEpoch primitives.Epoch) bool {
		// This value differs for our minimal and mainnet config which
		// is why we calculate it each time the policy is executed.
		withdrawableEpoch := corehelpers.ActivationExitEpoch(exitSubmissionEpoch) + params.BeaconConfig().MinValidatorWithdrawabilityDelay
		validWithdrawnEpoch := withdrawableEpoch + primitives.Epoch(params.BeaconConfig().MinGenesisActiveValidatorCount)/primitives.Epoch(params.BeaconConfig().MaxWithdrawalsPerPayload)
		requiredPolicy := policies.OnEpoch(validWithdrawnEpoch)
		return requiredPolicy(currentEpoch)
	},
	Evaluation: validatorsAreWithdrawn,
}

// ValidatorsVoteWithTheMajority verifies whether validator vote for executiondata using the majority algorithm.
var ValidatorsVoteWithTheMajority = e2etypes.Evaluator{
	Name:       "validators_vote_with_the_majority_%d",
	Policy:     policies.AfterNthEpoch(0),
	Evaluation: validatorsVoteWithTheMajority,
}

type mismatch struct {
	k [field_params.MLDSA87PubkeyLength]byte
	e uint64
	o uint64
}

func (m mismatch) String() string {
	return fmt.Sprintf("(%#x:%d:%d)", m.k, m.e, m.o)
}

func processesDepositsInBlocks(ec *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	expected := ec.Balances(e2etypes.PostGenesisDepositBatch)
	conn := conns[0]
	client := qrysmpb.NewBeaconChainClient(conn)
	chainHead, err := client.GetChainHead(context.Background(), &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}

	observed := make(map[[field_params.MLDSA87PubkeyLength]byte]uint64)
	for epoch := primitives.Epoch(0); epoch <= chainHead.HeadEpoch; epoch++ {
		req := &qrysmpb.ListBlocksRequest{QueryFilter: &qrysmpb.ListBlocksRequest_Epoch{Epoch: epoch}}
		blks, err := client.ListBeaconBlocks(context.Background(), req)
		if err != nil {
			return errors.Wrapf(err, "failed to get blocks for epoch %d from beacon-chain", epoch)
		}
		for _, blk := range blks.BlockContainers {
			sb, err := blocks.BeaconBlockContainerToSignedBeaconBlock(blk)
			if err != nil {
				return errors.Wrap(err, "failed to convert api response type to SignedBeaconBlock interface")
			}
			b := sb.Block()
			deposits := b.Body().Deposits()
			for _, d := range deposits {
				k := bytesutil.ToBytes2592(d.Data.PublicKey)
				v := observed[k]
				observed[k] = v + d.Data.Amount
			}
		}
	}
	mismatches := []string{}
	for k, ev := range expected {
		ov := observed[k]
		if ev != ov {
			mismatches = append(mismatches, mismatch{k: k, e: ev, o: ov}.String())
		}
	}
	if len(mismatches) != 0 {
		return fmt.Errorf("not all expected deposits observed on chain, len(expected)=%d, len(observed)=%d, mismatches=%d; details(key:expected:observed): %s", len(expected), len(observed), len(mismatches), strings.Join(mismatches, ","))
	}
	return nil
}

func verifyGraffitiInBlocks(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := qrysmpb.NewBeaconChainClient(conn)
	chainHead, err := client.GetChainHead(context.Background(), &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	begin := chainHead.HeadEpoch
	// Prevent underflow when this runs at epoch 0.
	if begin > 0 {
		begin = begin.Sub(1)
	}
	req := &qrysmpb.ListBlocksRequest{QueryFilter: &qrysmpb.ListBlocksRequest_Epoch{Epoch: begin}}
	blks, err := client.ListBeaconBlocks(context.Background(), req)
	if err != nil {
		return errors.Wrap(err, "failed to get blocks from beacon-chain")
	}
	for _, ctr := range blks.BlockContainers {
		blk, err := blocks.BeaconBlockContainerToSignedBeaconBlock(ctr)
		if err != nil {
			return err
		}
		var e bool
		slot := blk.Block().Slot()
		graffitiInBlock := blk.Block().Body().Graffiti()
		for _, graffiti := range helpers.Graffiti {
			if bytes.Equal(bytesutil.PadTo([]byte(graffiti), 32), graffitiInBlock[:]) {
				e = true
				break
			}
		}
		if !e && slot != 0 {
			return errors.New("could not get graffiti from the list")
		}
	}

	return nil
}

func activatesDepositedValidators(ec *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := qrysmpb.NewBeaconChainClient(conn)

	chainHead, err := client.GetChainHead(context.Background(), &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	epoch := chainHead.HeadEpoch

	validators, err := getAllValidators(client)
	if err != nil {
		return errors.Wrap(err, "failed to get validators")
	}
	expected := ec.Balances(e2etypes.PostGenesisDepositBatch)

	var deposits, lowBalance, wrongExit, wrongWithdraw int
	for _, v := range validators {
		key := bytesutil.ToBytes2592(v.PublicKey)
		if _, ok := expected[key]; !ok {
			continue
		}
		delete(expected, key)
		if v.ActivationEpoch != epoch {
			continue
		}
		deposits++
		if v.EffectiveBalance < params.BeaconConfig().MaxEffectiveBalance {
			lowBalance++
		}
		if v.ExitEpoch != params.BeaconConfig().FarFutureEpoch {
			wrongExit++
		}
		if v.WithdrawableEpoch != params.BeaconConfig().FarFutureEpoch {
			wrongWithdraw++
		}
	}

	// Make sure every post-genesis deposit has been proecssed, resulting in a validator.
	if len(expected) > 0 {
		return fmt.Errorf("missing %d validators for post-genesis deposits", len(expected))
	}

	if uint64(deposits) != params.BeaconConfig().MinPerEpochChurnLimit {
		return fmt.Errorf("expected %d deposits to be processed in epoch %d, received %d", params.BeaconConfig().MinPerEpochChurnLimit, epoch, deposits)
	}

	if lowBalance > 0 {
		return fmt.Errorf(
			"%d validators did not have genesis validator effective balance of %d",
			lowBalance,
			params.BeaconConfig().MaxEffectiveBalance,
		)
	} else if wrongExit > 0 {
		return fmt.Errorf("%d validators did not have an exit epoch of far future epoch", wrongExit)
	} else if wrongWithdraw > 0 {
		return fmt.Errorf("%d validators did not have a withdrawable epoch of far future epoch", wrongWithdraw)
	}
	return nil
}

func getAllValidators(c qrysmpb.BeaconChainClient) ([]*qrysmpb.Validator, error) {
	vals := make([]*qrysmpb.Validator, 0)
	pageToken := "0"
	for pageToken != "" {
		validatorRequest := &qrysmpb.ListValidatorsRequest{
			PageSize:  100,
			PageToken: pageToken,
		}
		validators, err := c.ListValidators(context.Background(), validatorRequest)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get validators")
		}
		for _, v := range validators.ValidatorList {
			vals = append(vals, v.Validator)
		}
		pageToken = validators.NextPageToken
	}
	return vals, nil
}

func depositedValidatorsAreActive(ec *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := qrysmpb.NewBeaconChainClient(conn)

	chainHead, err := client.GetChainHead(context.Background(), &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}

	vals, err := getAllValidators(client)
	if err != nil {
		return errors.Wrap(err, "error retrieving validator list from API")
	}
	inactive := 0
	lowBalance := 0
	nexits := 0
	expected := ec.Balances(e2etypes.PostGenesisDepositBatch)
	nexpected := len(expected)
	for _, v := range vals {
		key := bytesutil.ToBytes2592(v.PublicKey)
		if _, ok := expected[key]; !ok {
			continue // we aren't checking for this validator
		}
		// ignore voluntary exits when checking balance and active status
		exited := ec.ExitedVals[key]
		if exited {
			nexits++
			delete(expected, key)
			continue
		}
		if !corehelpers.IsActiveValidator(v, chainHead.HeadEpoch) {
			inactive++
		}
		if v.EffectiveBalance < params.BeaconConfig().MaxEffectiveBalance {
			lowBalance++
		}
		delete(expected, key)
	}
	if len(expected) > 0 {
		mk := make([]string, 0)
		for k := range expected {
			mk = append(mk, fmt.Sprintf("%#x", k))
		}
		return fmt.Errorf("API response missing %d validators, based on deposits; keys=%s", len(expected), strings.Join(mk, ","))
	}
	if inactive != 0 || lowBalance != 0 {
		return fmt.Errorf("active validator set does not match %d total deposited. %d exited, %d inactive, %d low balance", nexpected, nexits, inactive, lowBalance)
	}

	return nil
}

func proposeVoluntaryExit(ec *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	valClient := qrysmpb.NewBeaconNodeValidatorClient(conn)
	beaconClient := qrysmpb.NewBeaconChainClient(conn)

	ctx := context.Background()
	chainHead, err := beaconClient.GetChainHead(ctx, &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "could not get chain head")
	}

	validators, err := getAllValidators(beaconClient)
	if err != nil {
		return errors.Wrap(err, "could not get validators")
	}

	execIndices := []int{}
	for idx, val := range validators {
		withdrawalCredentials := val.GetWithdrawalCredentials()
		if len(withdrawalCredentials) == field_params.WithdrawalCredentialsLength {
			execIndices = append(execIndices, idx)
		}
	}
	if len(execIndices) > numOfExits {
		execIndices = execIndices[:numOfExits]
	}

	deposits, privKeys, err := util.DeterministicDepositsAndKeys(params.BeaconConfig().MinGenesisActiveValidatorCount)
	if err != nil {
		return err
	}

	var sendExit = func(exitedIndex primitives.ValidatorIndex) error {
		voluntaryExit := &qrysmpb.VoluntaryExit{
			Epoch:          chainHead.HeadEpoch,
			ValidatorIndex: exitedIndex,
		}
		req := &qrysmpb.DomainRequest{
			Epoch:  chainHead.HeadEpoch,
			Domain: params.BeaconConfig().DomainVoluntaryExit[:],
		}
		domain, err := valClient.DomainData(ctx, req)
		if err != nil {
			return err
		}
		signingData, err := signing.ComputeSigningRoot(voluntaryExit, domain.SignatureDomain)
		if err != nil {
			return err
		}
		signature := privKeys[exitedIndex].Sign(signingData[:])
		signedExit := &qrysmpb.SignedVoluntaryExit{
			Exit:      voluntaryExit,
			Signature: signature.Marshal(),
		}

		if _, err = valClient.ProposeExit(ctx, signedExit); err != nil {
			return errors.Wrap(err, "could not propose exit")
		}
		pubk := bytesutil.ToBytes2592(deposits[exitedIndex].Data.PublicKey)
		ec.ExitedVals[pubk] = true
		return nil
	}

	// Send exits for keys which already contain execution credentials.
	for _, idx := range execIndices {
		if err := sendExit(primitives.ValidatorIndex(idx)); err != nil {
			return err
		}
	}

	// Send an exit for a non-exited validator.
	for i := 0; i < numOfExits; {
		randIndex := primitives.ValidatorIndex(rand.Uint64() % params.BeaconConfig().MinGenesisActiveValidatorCount)
		if ec.ExitedVals[bytesutil.ToBytes2592(privKeys[randIndex].PublicKey().Marshal())] {
			continue
		}
		if err := sendExit(randIndex); err != nil {
			return err
		}
		i++
	}

	return nil
}

func validatorsHaveExited(ec *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := qrysmpb.NewBeaconChainClient(conn)
	for k := range ec.ExitedVals {
		validatorRequest := &qrysmpb.GetValidatorRequest{
			QueryFilter: &qrysmpb.GetValidatorRequest_PublicKey{
				PublicKey: k[:],
			},
		}
		validator, err := client.GetValidator(context.Background(), validatorRequest)
		if err != nil {
			return errors.Wrap(err, "failed to get validators")
		}
		if validator.ExitEpoch == params.BeaconConfig().FarFutureEpoch {
			return fmt.Errorf("expected validator %#x to be submitted for exit", k)
		}
	}
	return nil
}

func validatorsVoteWithTheMajority(ec *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := qrysmpb.NewBeaconChainClient(conn)
	chainHead, err := client.GetChainHead(context.Background(), &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}

	begin := chainHead.HeadEpoch
	// Prevent underflow when this runs at epoch 0.
	if begin > 0 {
		begin = begin.Sub(1)
	}
	req := &qrysmpb.ListBlocksRequest{QueryFilter: &qrysmpb.ListBlocksRequest_Epoch{Epoch: begin}}
	blks, err := client.ListBeaconBlocks(context.Background(), req)
	if err != nil {
		return errors.Wrap(err, "failed to get blocks from beacon-chain")
	}

	slotsPerVotingPeriod := params.E2ETestConfig().SlotsPerEpoch.Mul(uint64(params.E2ETestConfig().EpochsPerExecutionVotingPeriod))
	for _, blk := range blks.BlockContainers {
		var slot primitives.Slot
		var vote []byte
		switch blk.Block.(type) {
		case *qrysmpb.BeaconBlockContainer_ZondBlock:
			b := blk.GetZondBlock().Block
			slot = b.Slot
			vote = b.Body.ExecutionData.BlockHash
		case *qrysmpb.BeaconBlockContainer_BlindedZondBlock:
			b := blk.GetBlindedZondBlock().Block
			slot = b.Slot
			vote = b.Body.ExecutionData.BlockHash
		default:
			return errors.New("invalid block type")
		}
		ec.SeenVotes[slot] = vote

		// We treat epoch 1 differently from other epoch for two reasons:
		// - this evaluator is not executed for epoch 0 so we have to calculate the first slot differently
		// - for some reason the vote for the first slot in epoch 1 is 0x000... so we skip this slot
		var isFirstSlotInVotingPeriod bool
		if chainHead.HeadEpoch == 1 && slot%params.BeaconConfig().SlotsPerEpoch == 0 {
			continue
		}
		// We skipped the first slot so we treat the second slot as the starting slot of epoch 1.
		if chainHead.HeadEpoch == 1 {
			isFirstSlotInVotingPeriod = slot%params.BeaconConfig().SlotsPerEpoch == 1
		} else {
			isFirstSlotInVotingPeriod = slot%slotsPerVotingPeriod == 0
		}
		if isFirstSlotInVotingPeriod {
			ec.ExpectedExecutionDataVote = vote
			return nil
		}

		if !bytes.Equal(vote, ec.ExpectedExecutionDataVote) {
			for i := primitives.Slot(0); i < slot; i++ {
				v, ok := ec.SeenVotes[i]
				if ok {
					fmt.Printf("vote at slot=%d = %#x\n", i, v)
				} else {
					fmt.Printf("did not see slot=%d\n", i)
				}
			}
			return fmt.Errorf("incorrect executiondata vote for slot %d; expected: %#x vs voted: %#x",
				slot, ec.ExpectedExecutionDataVote, vote)
		}
	}
	return nil
}

func validatorsAreWithdrawn(ec *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	beaconClient := qrysmpb.NewBeaconChainClient(conn)
	debugClient := qrysmpb.NewDebugClient(conn)

	ctx := context.Background()
	chainHead, err := beaconClient.GetChainHead(ctx, &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "could not get chain head")
	}
	stObj, err := debugClient.GetBeaconState(ctx, &qrysmpb.BeaconStateRequest{QueryFilter: &qrysmpb.BeaconStateRequest_Slot{Slot: chainHead.HeadSlot}})
	if err != nil {
		return errors.Wrap(err, "could not get state object")
	}
	versionedMarshaler, err := detect.FromState(stObj.Encoded)
	if err != nil {
		return errors.Wrap(err, "could not get state marshaler")
	}
	st, err := versionedMarshaler.UnmarshalBeaconState(stObj.Encoded)
	if err != nil {
		return errors.Wrap(err, "could not get state")
	}

	for key := range ec.ExitedVals {
		valIdx, ok := st.ValidatorIndexByPubkey(key)
		if !ok {
			return errors.Errorf("pubkey %#x does not exist in our state", key)
		}
		bal, err := st.BalanceAtIndex(valIdx)
		if err != nil {
			return err
		}
		// Only return an error if the validator has more than 1 quanta
		// in its balance.
		if bal > 1*params.BeaconConfig().ShorPerQuanta {
			return errors.Errorf("Validator index %d with key %#x hasn't withdrawn. Their balance is %d.", valIdx, key, bal)
		}
	}
	return nil
}
