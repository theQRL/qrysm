package sync

import (
	"context"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/v4/monitoring/tracing"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"go.opencensus.io/trace"
)

func (s *Service) validateDilithiumToExecutionChange(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	// Validation runs on publish (not just subscriptions), so we should approve any message from
	// ourselves.
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// The head state will be too far away to validate any execution change.
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validateDilithiumToExecutionChange")
	defer span.End()

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}

	dilithiumChange, ok := m.(*zondpb.SignedDilithiumToExecutionChange)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}

	// Check that the validator hasn't submitted a previous execution change.
	if s.cfg.dilithiumToExecPool.ValidatorExists(dilithiumChange.Message.ValidatorIndex) {
		return pubsub.ValidationIgnore, nil
	}
	st, err := s.cfg.chain.HeadStateReadOnly(ctx)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	// Validate that the execution change object is valid.
	_, err = blocks.ValidateDilithiumToExecutionChange(st, dilithiumChange)
	if err != nil {
		return pubsub.ValidationReject, err
	}
	// Validate the signature of the message using our batch gossip verifier.
	sigBatch, err := blocks.DilithiumChangesSignatureBatch(st, []*zondpb.SignedDilithiumToExecutionChange{dilithiumChange})
	if err != nil {
		return pubsub.ValidationReject, err
	}
	res, err := s.validateWithBatchVerifier(ctx, "dilithium to execution change", sigBatch)
	if res != pubsub.ValidationAccept {
		return res, err
	}
	msg.ValidatorData = dilithiumChange // Used in downstream subscriber
	return pubsub.ValidationAccept, nil
}
