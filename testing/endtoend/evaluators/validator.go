package evaluators

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	zondpbservice "github.com/theQRL/qrysm/v4/proto/zond/service"
	v2 "github.com/theQRL/qrysm/v4/proto/zond/v2"
	"github.com/theQRL/qrysm/v4/testing/endtoend/policies"
	"github.com/theQRL/qrysm/v4/testing/endtoend/types"
	"github.com/theQRL/qrysm/v4/time/slots"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

var expectedParticipation = 0.99

var expectedSyncParticipation = 0.99

// ValidatorsAreActive ensures the expected amount of validators are active.
var ValidatorsAreActive = types.Evaluator{
	Name:       "validators_active_epoch_%d",
	Policy:     policies.AllEpochs,
	Evaluation: validatorsAreActive,
}

// ValidatorsParticipatingAtEpoch ensures the expected amount of validators are participating.
var ValidatorsParticipatingAtEpoch = func(epoch primitives.Epoch) types.Evaluator {
	return types.Evaluator{
		Name:       "validators_participating_epoch_%d",
		Policy:     policies.AfterNthEpoch(epoch),
		Evaluation: validatorsParticipating,
	}
}

// ValidatorSyncParticipation ensures the expected amount of sync committee participants
// are active.
var ValidatorSyncParticipation = types.Evaluator{
	Name: "validator_sync_participation_%d",
	// Policy:     policies.OnwardsNthEpoch(helpers.AltairE2EForkEpoch),
	Policy:     policies.OnwardsNthEpoch(6),
	Evaluation: validatorsSyncParticipation,
}

func validatorsAreActive(ec *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := zondpb.NewBeaconChainClient(conn)
	// Balances actually fluctuate but we just want to check initial balance.
	validatorRequest := &zondpb.ListValidatorsRequest{
		PageSize: int32(params.BeaconConfig().MinGenesisActiveValidatorCount),
		Active:   true,
	}
	validators, err := client.ListValidators(context.Background(), validatorRequest)
	if err != nil {
		return errors.Wrap(err, "failed to get validators")
	}

	expectedCount := params.BeaconConfig().MinGenesisActiveValidatorCount
	receivedCount := uint64(len(validators.ValidatorList))
	if expectedCount != receivedCount {
		return fmt.Errorf("expected validator count to be %d, received %d", expectedCount, receivedCount)
	}

	effBalanceLowCount := 0
	exitEpochWrongCount := 0
	withdrawEpochWrongCount := 0
	for _, item := range validators.ValidatorList {
		if ec.ExitedVals[bytesutil.ToBytes2592(item.Validator.PublicKey)] {
			continue
		}
		if item.Validator.EffectiveBalance < params.BeaconConfig().MaxEffectiveBalance {
			effBalanceLowCount++
		}
		if item.Validator.ExitEpoch != params.BeaconConfig().FarFutureEpoch {
			exitEpochWrongCount++
		}
		if item.Validator.WithdrawableEpoch != params.BeaconConfig().FarFutureEpoch {
			withdrawEpochWrongCount++
		}
	}

	if effBalanceLowCount > 0 {
		return fmt.Errorf(
			"%d validators did not have genesis validator effective balance of %d",
			effBalanceLowCount,
			params.BeaconConfig().MaxEffectiveBalance,
		)
	} else if exitEpochWrongCount > 0 {
		return fmt.Errorf("%d validators did not have genesis validator exit epoch of far future epoch", exitEpochWrongCount)
	} else if withdrawEpochWrongCount > 0 {
		return fmt.Errorf("%d validators did not have genesis validator withdrawable epoch of far future epoch", withdrawEpochWrongCount)
	}

	return nil
}

// validatorsParticipating ensures the validators have an acceptable participation rate.
func validatorsParticipating(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := zondpb.NewBeaconChainClient(conn)
	debugClient := zondpbservice.NewBeaconDebugClient(conn)
	validatorRequest := &zondpb.GetValidatorParticipationRequest{}
	participation, err := client.GetValidatorParticipation(context.Background(), validatorRequest)
	if err != nil {
		return errors.Wrap(err, "failed to get validator participation")
	}

	partRate := participation.Participation.GlobalParticipationRate
	expected := float32(expectedParticipation)

	if partRate < expected {
		st, err := debugClient.GetBeaconStateV2(context.Background(), &v2.BeaconStateRequestV2{StateId: []byte("head")})
		if err != nil {
			return errors.Wrap(err, "failed to get beacon state")
		}
		var missSrcVals []uint64
		var missTgtVals []uint64
		var missHeadVals []uint64
		switch obj := st.Data.State.(type) {
		case *v2.BeaconStateContainer_CapellaState:
			missSrcVals, missTgtVals, missHeadVals, err = findMissingValidators(obj.CapellaState.PreviousEpochParticipation)
			if err != nil {
				return errors.Wrap(err, "failed to get missing validators")
			}
		default:
			return fmt.Errorf("unrecognized version: %v", st.Version)
		}
		return fmt.Errorf(
			"validator participation was below for epoch %d, expected %f, received: %f."+
				" Missing Source,Target and Head validators are %v, %v, %v",
			participation.Epoch,
			expected,
			partRate,
			missSrcVals,
			missTgtVals,
			missHeadVals,
		)
	}
	return nil
}

// validatorsSyncParticipation ensures the validators have an acceptable participation rate for
// sync committee assignments.
func validatorsSyncParticipation(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := zondpb.NewNodeClient(conn)
	altairClient := zondpb.NewBeaconChainClient(conn)
	genesis, err := client.GetGenesis(context.Background(), &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get genesis data")
	}
	currSlot := slots.CurrentSlot(uint64(genesis.GenesisTime.AsTime().Unix()))
	currEpoch := slots.ToEpoch(currSlot)
	lowestBound := primitives.Epoch(0)
	if currEpoch >= 1 {
		lowestBound = currEpoch - 1
	}

	// if lowestBound < helpers.AltairE2EForkEpoch {
	// 	lowestBound = helpers.AltairE2EForkEpoch
	// }

	blockCtrs, err := altairClient.ListBeaconBlocks(context.Background(), &zondpb.ListBlocksRequest{QueryFilter: &zondpb.ListBlocksRequest_Epoch{Epoch: lowestBound}})
	if err != nil {
		return errors.Wrap(err, "failed to get validator participation")
	}
	for _, ctr := range blockCtrs.BlockContainers {
		b, err := syncCompatibleBlockFromCtr(ctr)
		if err != nil {
			return errors.Wrapf(err, "block type doesn't exist for block at epoch %d", lowestBound)
		}

		if b.IsNil() {
			return errors.New("nil block provided")
		}
		// forkStartSlot, err := slots.EpochStart(helpers.AltairE2EForkEpoch)
		// if err != nil {
		// 	return err
		// }
		// if forkStartSlot == b.Block().Slot() {
		// 	// Skip fork slot.
		// 	continue
		// }
		expectedParticipation := expectedSyncParticipation
		// switch slots.ToEpoch(b.Block().Slot()) {
		// case helpers.AltairE2EForkEpoch:
		// 	// Drop expected sync participation figure.
		// 	expectedParticipation = 0.90
		// default:
		// 	// no-op
		// }
		syncAgg, err := b.Block().Body().SyncAggregate()
		if err != nil {
			return err
		}
		threshold := uint64(float64(syncAgg.SyncCommitteeBits.Len()) * expectedParticipation)
		if syncAgg.SyncCommitteeBits.Count() < threshold {
			return errors.Errorf("In block of slot %d ,the aggregate bitvector with length of %d only got a count of %d", b.Block().Slot(), threshold, syncAgg.SyncCommitteeBits.Count())
		}
	}
	if lowestBound == currEpoch {
		return nil
	}
	blockCtrs, err = altairClient.ListBeaconBlocks(context.Background(), &zondpb.ListBlocksRequest{QueryFilter: &zondpb.ListBlocksRequest_Epoch{Epoch: currEpoch}})
	if err != nil {
		return errors.Wrap(err, "failed to get validator participation")
	}
	for _, ctr := range blockCtrs.BlockContainers {
		b, err := syncCompatibleBlockFromCtr(ctr)
		if err != nil {
			return errors.Wrapf(err, "block type doesn't exist for block at epoch %d", lowestBound)
		}

		if b.IsNil() {
			return errors.New("nil block provided")
		}
		// forkSlot, err := slots.EpochStart(helpers.AltairE2EForkEpoch)
		// if err != nil {
		// 	return err
		// }
		// nexForkSlot, err := slots.EpochStart(helpers.BellatrixE2EForkEpoch)
		// if err != nil {
		// 	return err
		// }
		// switch b.Block().Slot() {
		// case forkSlot, forkSlot + 1, nexForkSlot:
		// 	// Skip evaluation of the slot.
		// 	continue
		// default:
		// 	// no-op
		// }
		syncAgg, err := b.Block().Body().SyncAggregate()
		if err != nil {
			return err
		}
		threshold := uint64(float64(syncAgg.SyncCommitteeBits.Len()) * expectedSyncParticipation)
		if syncAgg.SyncCommitteeBits.Count() < threshold {
			return errors.Errorf("In block of slot %d ,the aggregate bitvector with length of %d only got a count of %d", b.Block().Slot(), threshold, syncAgg.SyncCommitteeBits.Count())
		}
	}
	return nil
}

func syncCompatibleBlockFromCtr(container *zondpb.BeaconBlockContainer) (interfaces.ReadOnlySignedBeaconBlock, error) {
	if container.GetCapellaBlock() != nil {
		return blocks.NewSignedBeaconBlock(container.GetCapellaBlock())
	}
	if container.GetBlindedCapellaBlock() != nil {
		return blocks.NewSignedBeaconBlock(container.GetBlindedCapellaBlock())
	}
	return nil, errors.New("no supported block type in container")
}

func findMissingValidators(participation []byte) ([]uint64, []uint64, []uint64, error) {
	cfg := params.BeaconConfig()
	sourceFlagIndex := cfg.TimelySourceFlagIndex
	targetFlagIndex := cfg.TimelyTargetFlagIndex
	headFlagIndex := cfg.TimelyHeadFlagIndex
	var missingSourceValidators []uint64
	var missingHeadValidators []uint64
	var missingTargetValidators []uint64
	for i, b := range participation {
		hasSource, err := altair.HasValidatorFlag(b, sourceFlagIndex)
		if err != nil {
			return nil, nil, nil, err
		}
		if !hasSource {
			missingSourceValidators = append(missingSourceValidators, uint64(i))
		}
		hasTarget, err := altair.HasValidatorFlag(b, targetFlagIndex)
		if err != nil {
			return nil, nil, nil, err
		}
		if !hasTarget {
			missingTargetValidators = append(missingTargetValidators, uint64(i))
		}
		hasHead, err := altair.HasValidatorFlag(b, headFlagIndex)
		if err != nil {
			return nil, nil, nil, err
		}
		if !hasHead {
			missingHeadValidators = append(missingHeadValidators, uint64(i))
		}
	}
	return missingSourceValidators, missingTargetValidators, missingHeadValidators, nil
}
