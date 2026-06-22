package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/cmd/validator/flags"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/time/slots"
	"github.com/theQRL/qrysm/validator/client/iface"
	"go.opencensus.io/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// time to wait before trying to reconnect with beacon node.
var backOffPeriod = 10 * time.Second

// Run the main validator routine. This routine exits if the context is
// canceled.
//
// Order of operations:
// 1 - Initialize validator data
// 2 - Wait for validator activation
// 3 - Wait for the next slot start
// 4 - Update assignments
// 5 - Determine role at current slot
// 6 - Perform assigned role, if any
func run(ctx context.Context, v iface.Validator) error {
	cleanup := v.Done
	defer cleanup()
	runnerCtx, runnerCancel := context.WithCancel(ctx)
	defer runnerCancel()

	currentSlot, err := initializeValidatorAndGetCurrentSlot(runnerCtx, v)
	if err != nil {
		if runnerCtx.Err() != nil {
			return nil
		}
		return err
	}

	connectionErrorChannel := make(chan error, 1)
	go v.ReceiveBlocks(runnerCtx, connectionErrorChannel)
	if err := v.UpdateDuties(runnerCtx, currentSlot); err != nil {
		if isConnectionError(err) {
			return err
		}
		handleAssignmentError(err, currentSlot)
	}

	accountsChangedChan := make(chan [][field_params.MLDSA87PubkeyLength]byte, 1)
	km, err := v.Keymanager()
	if err != nil {
		return errors.Wrap(err, "could not get keymanager")
	}
	sub := km.SubscribeAccountChanges(accountsChangedChan)
	defer close(accountsChangedChan)
	defer sub.Unsubscribe()
	// check if proposer settings is still nil
	// Set properties on the beacon node like the fee recipient for validators that are being used & active.
	if v.ProposerSettings() != nil {
		log.Infof("Validator client started with provided proposer settings that sets options such as fee recipient"+
			" and will periodically update the beacon node and custom builder (if --%s)", flags.EnableBuilderFlag.Name)
		if err := v.PushProposerSettings(runnerCtx, km, currentSlot); err != nil {
			log.WithError(err).Warn("Failed to update proposer settings on startup, will retry on next epoch")
		}
	} else {
		log.Warn("Validator client started without proposer settings such as fee recipient" +
			" and will continue to use settings provided in the beacon node.")
	}

	// Start the slot ticker now that all blocking initialization (chain start,
	// keymanager init, sync, activation, doppelganger check, duties, proposer
	// settings) is done. Starting it earlier risks replaying old ticks the
	// first time the loop reads from v.NextSlot().
	v.SetTicker()

	for {
		_, cancel := context.WithCancel(runnerCtx)
		ctx, span := trace.StartSpan(runnerCtx, "validator.processSlot")

		select {
		case <-ctx.Done():
			log.Info("Context canceled, stopping validator")
			span.End()
			cancel()
			//nolint:govet
			return nil // Exit if context is canceled.
		case blocksError := <-connectionErrorChannel:
			span.End()
			cancel()
			if blocksError != nil {
				log.WithError(blocksError).Warn("block stream interrupted")
				return blocksError
			}
		case currentKeys := <-accountsChangedChan:
			span.End()
			cancel()
			onAccountsChanged(ctx, v, currentKeys, accountsChangedChan)
		case slot := <-v.NextSlot():
			span.AddAttributes(trace.Int64Attribute("slot", int64(slot))) // lint:ignore uintcast -- This conversion is OK for tracing.

			deadline := v.SlotDeadline(slot)
			slotCtx, cancel := context.WithDeadline(ctx, deadline) //nolint:govet
			log := log.WithField("slot", slot)
			log.WithField("deadline", deadline).Debug("Set deadline for proposals and attestations")

			// Keep trying to update assignments if they are nil or if we are past an
			// epoch transition in the beacon node's state.
			if err := v.UpdateDuties(ctx, slot); err != nil {
				if isConnectionError(err) {
					cancel()
					span.End()
					return err
				}
				handleAssignmentError(err, slot)
				cancel()
				span.End()
				continue
			}

			// call push proposer setting at the start of each epoch to account for the following edge case:
			// proposer is activated at the start of epoch and tries to propose immediately
			if slots.IsEpochStart(slot) && v.ProposerSettings() != nil {
				go func() {
					if err := v.PushProposerSettings(ctx, km, slot); err != nil {
						log.WithError(err).Warn("Failed to update proposer settings")
					}
				}()
			}

			// Start fetching domain data for the next epoch on a context
			// independent of slotCtx but bounded by the slot deadline, so the
			// 8 RPC fetches self-terminate at the slot boundary instead of
			// piling up across epochs under network stalls. (upstream PR
			// #15268)
			if slots.IsEpochEnd(slot) {
				domainCtx, _ := context.WithDeadline(ctx, deadline) //nolint:govet
				go v.UpdateDomainDataCaches(domainCtx, slot+1)
			}

			var wg sync.WaitGroup

			allRoles, err := v.RolesAt(ctx, slot)
			if err != nil {
				if isConnectionError(err) {
					cancel()
					span.End()
					return err
				}
				log.WithError(err).Error("Could not get validator roles")
				cancel()
				span.End()
				continue
			}
			performRoles(slotCtx, allRoles, v, slot, &wg, span, cancel)
		}
	}
}

func runWithRecovery(ctx context.Context, v iface.Validator, waitForRecovery func(context.Context) error) {
	if waitForRecovery == nil {
		waitForRecovery = waitForRetry
	}
	for {
		if ctx.Err() != nil {
			return
		}
		runnerCtx, cancel := context.WithCancel(ctx)
		err := run(runnerCtx, v)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.WithError(err).Warn("Validator runner stopped, waiting for recovery")
		} else {
			log.Warn("Validator runner stopped unexpectedly, waiting for recovery")
		}
		if err := waitForRecovery(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.WithError(err).Warn("Could not wait for validator runner recovery")
			return
		}
	}
}

func onAccountsChanged(ctx context.Context, v iface.Validator, current [][field_params.MLDSA87PubkeyLength]byte, ac chan [][field_params.MLDSA87PubkeyLength]byte) {
	anyActive, err := v.HandleKeyReload(ctx, current)
	if err != nil {
		log.WithError(err).Error("Could not properly handle reloaded keys")
	}
	if !anyActive {
		log.Warn("No active keys found. Waiting for activation...")
		err := v.WaitForActivation(ctx, ac)
		if err != nil {
			log.WithError(err).Warn("Could not wait for validator activation")
		}
	}
}

func initializeValidatorAndGetCurrentSlot(ctx context.Context, v iface.Validator) (primitives.Slot, error) {
	ticker := time.NewTicker(backOffPeriod)
	defer ticker.Stop()

	firstTime := true
	for {
		if !firstTime {
			if ctx.Err() != nil {
				log.Info("Context canceled, stopping validator")
				return 0, errors.New("context canceled")
			}
			<-ticker.C
		} else {
			firstTime = false
		}
		err := v.WaitForChainStart(ctx)
		if isConnectionError(err) {
			log.WithError(err).Warn("Could not determine if beacon chain started")
			continue
		}
		if err != nil {
			return 0, errors.Wrap(err, "could not determine if beacon chain started")
		}

		err = v.WaitForKeymanagerInitialization(ctx)
		if err != nil {
			return 0, errors.Wrap(err, "wallet is not ready")
		}

		err = v.WaitForSync(ctx)
		if isConnectionError(err) {
			log.WithError(err).Warn("Could not determine if beacon chain started")
			continue
		}
		if err != nil {
			return 0, errors.Wrap(err, "could not determine if beacon node synced")
		}
		err = v.WaitForActivation(ctx, nil /* accountsChangedChan */)
		if err != nil {
			return 0, errors.Wrap(err, "could not wait for validator activation")
		}
		err = v.CheckDoppelGanger(ctx)
		if isConnectionError(err) {
			log.WithError(err).Warn("Could not wait for checking doppelganger")
			continue
		}
		if err != nil {
			return 0, errors.Wrap(err, "could not succeed with doppelganger check")
		}
		break
	}
	return slots.CurrentSlot(v.GenesisTime()), nil
}

func waitForRetry(ctx context.Context) error {
	timer := time.NewTimer(backOffPeriod)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func performRoles(slotCtx context.Context, allRoles map[[field_params.MLDSA87PubkeyLength]byte][]iface.ValidatorRole, v iface.Validator, slot primitives.Slot, wg *sync.WaitGroup, span *trace.Span, cancel context.CancelFunc) {
	for pubKey, roles := range allRoles {
		wg.Add(len(roles))
		for _, role := range roles {
			go func(role iface.ValidatorRole, pubKey [field_params.MLDSA87PubkeyLength]byte) {
				defer wg.Done()
				switch role {
				case iface.RoleAttester:
					v.SubmitAttestation(slotCtx, slot, pubKey)
				case iface.RoleProposer:
					v.ProposeBlock(slotCtx, slot, pubKey)
				case iface.RoleAggregator:
					v.SubmitAggregateAndProof(slotCtx, slot, pubKey)
				case iface.RoleSyncCommittee:
					v.SubmitSyncCommitteeMessage(slotCtx, slot, pubKey)
				case iface.RoleSyncCommitteeAggregator:
					v.SubmitSignedContributionAndProof(slotCtx, slot, pubKey)
				case iface.RoleUnknown:
					log.WithField("pubKey", fmt.Sprintf("%#x", bytesutil.Trunc(pubKey[:]))).Trace("No active roles, doing nothing")
				default:
					log.Warnf("Unhandled role %v", role)
				}
			}(role, pubKey)
		}
	}

	// Wait for all processes to complete, then report span complete.
	go func() {
		wg.Wait()
		defer cancel()
		defer span.End()
		defer func() {
			if err := recover(); err != nil { // catch any panic in logging
				log.WithField("err", err).
					Error("Panic occurred when logging validator report. This" +
						" should never happen! Please file a report at github.com/theQRL/qrysm/issues/new")
			}
		}()
		// Log this client performance in the previous epoch
		v.LogAttestationsSubmitted()
		v.LogSyncCommitteeMessagesSubmitted()
		if err := v.LogValidatorGainsAndLosses(slotCtx, slot); err != nil {
			log.WithError(err).Error("Could not report validator's rewards/penalties")
		}
	}()
}

func isConnectionError(err error) bool {
	return err != nil && errors.Is(err, iface.ErrConnectionIssue)
}

func handleAssignmentError(err error, slot primitives.Slot) {
	if errors.Is(err, ErrValidatorsAllExited) {
		log.Warn(ErrValidatorsAllExited)
	} else if errCode, ok := status.FromError(err); ok && errCode.Code() == codes.NotFound {
		log.WithField(
			"epoch", slot/params.BeaconConfig().SlotsPerEpoch,
		).Warn("Validator not yet assigned to epoch")
	} else {
		log.WithField("error", err).Error("Failed to update assignments")
	}
}
