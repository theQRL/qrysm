package blockchain

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
)

type weakSubjectivityDB interface {
	HasBlock(ctx context.Context, blockRoot [32]byte) bool
	IsFinalizedBlock(ctx context.Context, blockRoot [32]byte) bool
	HighestRootsBelowSlot(ctx context.Context, slot primitives.Slot) (primitives.Slot, [][32]byte, error)
}

type WeakSubjectivityVerifier struct {
	enabled  bool
	verified bool
	root     [32]byte
	epoch    primitives.Epoch
	slot     primitives.Slot
	db       weakSubjectivityDB
}

// NewWeakSubjectivityVerifier validates a checkpoint, and if valid, uses it to initialize a weak subjectivity verifier.
func NewWeakSubjectivityVerifier(wsc *qrysmpb.Checkpoint, db weakSubjectivityDB) (*WeakSubjectivityVerifier, error) {
	if wsc == nil || len(wsc.Root) == 0 || wsc.Epoch == 0 {
		log.Debug("--weak-subjectivity-checkpoint not provided")
		return &WeakSubjectivityVerifier{
			enabled: false,
		}, nil
	}
	startSlot, err := slots.EpochStart(wsc.Epoch)
	if err != nil {
		return nil, err
	}
	return &WeakSubjectivityVerifier{
		enabled:  true,
		verified: false,
		root:     bytesutil.ToBytes32(wsc.Root),
		epoch:    wsc.Epoch,
		db:       db,
		slot:     startSlot,
	}, nil
}

// VerifyWeakSubjectivity verifies the weak subjectivity root in the service struct.
// Reference design: https://github.com/ethereum/consensus-specs/blob/master/specs/phase0/weak-subjectivity.md#weak-subjectivity-sync-procedure
func (v *WeakSubjectivityVerifier) VerifyWeakSubjectivity(ctx context.Context, finalizedEpoch primitives.Epoch) error {
	if v.verified || !v.enabled {
		return nil
	}
	// Wait until finality has advanced past the weak subjectivity epoch: the
	// finalized block roots index is only guaranteed canonical-exact for epochs
	// strictly older than the latest finalized epoch.
	if v.epoch >= finalizedEpoch {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	log.Infof("Performing weak subjectivity check for root %#x in epoch %d", v.root, v.epoch)

	if !v.db.HasBlock(ctx, v.root) {
		return errors.Wrap(errWSBlockNotFound, fmt.Sprintf("missing root %#x", v.root))
	}
	// The block must be part of the finalized canonical chain — an orphaned
	// fork block stored in the DB must not satisfy the check.
	if !v.db.IsFinalizedBlock(ctx, v.root) {
		return errors.Wrap(errWSBlockNotCanonical, fmt.Sprintf("root=%#x, epoch=%d", v.root, v.epoch))
	}
	// A checkpoint names the last canonical block at or before its epoch's
	// first slot. Skipped slots (or whole epochs) can put it in an older epoch.
	before, err := v.slot.SafeAdd(1)
	if err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		slot, roots, err := v.db.HighestRootsBelowSlot(ctx, before)
		if err != nil {
			return errors.Wrap(err, "could not find weak subjectivity checkpoint block")
		}
		for _, root := range roots {
			if !v.db.IsFinalizedBlock(ctx, root) {
				continue
			}
			if root != v.root {
				return errors.Wrap(errWSCheckpointMismatch, fmt.Sprintf("expected=%#x, canonical=%#x, epoch=%d", v.root, root, v.epoch))
			}
			v.verified = true
			log.Info("Weak subjectivity check has passed")
			return nil
		}
		if slot == 0 {
			return errors.Wrap(errWSBlockNotCanonical, fmt.Sprintf("root=%#x, epoch=%d", v.root, v.epoch))
		}
		// A stored orphan at a skipped canonical slot must not hide the
		// earlier finalized block used by this checkpoint.
		before = slot
	}
}

// verifyWeakSubjectivity enforces the configured trust anchor once persisted
// finality allows verification. Local read failures remain retryable. The caller
// must hold the forkchoice lock, which also serializes the verifier's state.
func (s *Service) verifyWeakSubjectivity(ctx context.Context, epoch primitives.Epoch) error {
	if s.wsVerifier == nil {
		return nil
	}
	err := s.wsVerifier.VerifyWeakSubjectivity(ctx, epoch)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, errWSBlockNotFound) || errors.Is(err, errWSBlockNotCanonical) || errors.Is(err, errWSCheckpointMismatch) {
		log.WithError(err).Fatal("Could not verify weak subjectivity checkpoint")
	}
	return err
}
