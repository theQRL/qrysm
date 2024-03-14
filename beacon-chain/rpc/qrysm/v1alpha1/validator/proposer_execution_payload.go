package validator

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/time"
	"github.com/theQRL/qrysm/v4/beacon-chain/db/kv"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	payloadattribute "github.com/theQRL/qrysm/v4/consensus-types/payload-attribute"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/time/slots"
	"go.opencensus.io/trace"
)

var (
	// payloadIDCacheMiss tracks the number of payload ID requests that aren't present in the cache.
	payloadIDCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "payload_id_cache_miss",
		Help: "The number of payload id get requests that aren't present in the cache.",
	})
	// payloadIDCacheHit tracks the number of payload ID requests that are present in the cache.
	payloadIDCacheHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "payload_id_cache_hit",
		Help: "The number of payload id get requests that are present in the cache.",
	})
)

// This returns the local execution payload of a given slot. The function has full awareness of pre and post merge.
func (vs *Server) getLocalPayload(ctx context.Context, blk interfaces.ReadOnlyBeaconBlock, st state.BeaconState) (interfaces.ExecutionData, bool, error) {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.getLocalPayload")
	defer span.End()

	slot := blk.Slot()
	vIdx := blk.ProposerIndex()
	headRoot := blk.ParentRoot()
	proposerID, payloadId, ok := vs.ProposerSlotIndexCache.GetProposerPayloadIDs(slot, headRoot)
	feeRecipient := params.BeaconConfig().DefaultFeeRecipient
	recipient, err := vs.BeaconDB.FeeRecipientByValidatorID(ctx, vIdx)
	switch err == nil {
	case true:
		feeRecipient = recipient
	case errors.As(err, kv.ErrNotFoundFeeRecipient):
		// If fee recipient is not found in DB and not set from beacon node CLI,
		// use the burn address.
		if feeRecipient.String() == params.BeaconConfig().EthBurnAddressHex {
			logrus.WithFields(logrus.Fields{
				"validatorIndex": vIdx,
				"burnAddress":    params.BeaconConfig().EthBurnAddressHex,
			}).Warn("Fee recipient is currently using the burn address, " +
				"you will not be rewarded transaction fees on this setting. " +
				"Please set a different zond address as the fee recipient. " +
				"Please refer to our documentation for instructions")
		}
	default:
		return nil, false, errors.Wrap(err, "could not get fee recipient in db")
	}

	if ok && proposerID == vIdx && payloadId != [8]byte{} { // Payload ID is cache hit. Return the cached payload ID.
		var pid [8]byte
		copy(pid[:], payloadId[:])
		payloadIDCacheHit.Inc()
		payload, overrideBuilder, err := vs.ExecutionEngineCaller.GetPayload(ctx, pid, slot)
		switch {
		case err == nil:
			warnIfFeeRecipientDiffers(payload, feeRecipient)
			return payload, overrideBuilder, nil
		case errors.Is(err, context.DeadlineExceeded):
		default:
			return nil, false, errors.Wrap(err, "could not get cached payload from execution client")
		}
	}

	parentHash, err := vs.getParentBlockHash(ctx, st, slot)
	if err != nil {
		return nil, false, err
	}
	payloadIDCacheMiss.Inc()

	random, err := helpers.RandaoMix(st, time.CurrentEpoch(st))
	if err != nil {
		return nil, false, err
	}

	finalizedBlockHash := vs.FinalizationFetcher.FinalizedBlockHash()
	justifiedBlockHash := vs.FinalizationFetcher.UnrealizedJustifiedPayloadBlockHash()

	f := &enginev1.ForkchoiceState{
		HeadBlockHash:      parentHash,
		SafeBlockHash:      justifiedBlockHash[:],
		FinalizedBlockHash: finalizedBlockHash[:],
	}

	t, err := slots.ToTime(st.GenesisTime(), slot)
	if err != nil {
		return nil, false, err
	}
	var attr payloadattribute.Attributer
	switch st.Version() {
	case version.Capella:
		withdrawals, err := st.ExpectedWithdrawals()
		if err != nil {
			return nil, false, err
		}
		attr, err = payloadattribute.New(&enginev1.PayloadAttributesV2{
			Timestamp:             uint64(t.Unix()),
			PrevRandao:            random,
			SuggestedFeeRecipient: feeRecipient.Bytes(),
			Withdrawals:           withdrawals,
		})
		if err != nil {
			return nil, false, err
		}
	default:
		return nil, false, errors.New("unknown beacon state version")
	}
	payloadID, _, err := vs.ExecutionEngineCaller.ForkchoiceUpdated(ctx, f, attr)
	if err != nil {
		return nil, false, errors.Wrap(err, "could not prepare payload")
	}
	if payloadID == nil {
		return nil, false, fmt.Errorf("nil payload with block hash: %#x", parentHash)
	}
	payload, overrideBuilder, err := vs.ExecutionEngineCaller.GetPayload(ctx, *payloadID, slot)
	if err != nil {
		return nil, false, err
	}
	warnIfFeeRecipientDiffers(payload, feeRecipient)
	return payload, overrideBuilder, nil
}

// warnIfFeeRecipientDiffers logs a warning if the fee recipient in the included payload does not
// match the requested one.
func warnIfFeeRecipientDiffers(payload interfaces.ExecutionData, feeRecipient common.Address) {
	// Warn if the fee recipient is not the value we expect.
	if payload != nil && !bytes.Equal(payload.FeeRecipient(), feeRecipient[:]) {
		logrus.WithFields(logrus.Fields{
			"wantedFeeRecipient": fmt.Sprintf("%#x", feeRecipient),
			"received":           fmt.Sprintf("%#x", payload.FeeRecipient()),
		}).Warn("Fee recipient address from execution client is not what was expected. " +
			"It is possible someone has compromised your client to try and take your transaction fees")
	}
}

func (vs *Server) getBuilderPayload(ctx context.Context,
	slot primitives.Slot,
	vIdx primitives.ValidatorIndex) (interfaces.ExecutionData, error) {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.getBuilderPayload")
	defer span.End()

	canUseBuilder, err := vs.canUseBuilder(ctx, slot, vIdx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check if we can use the builder")
	}
	span.AddAttributes(trace.BoolAttribute("canUseBuilder", canUseBuilder))
	if !canUseBuilder {
		return nil, nil
	}

	return vs.getPayloadHeaderFromBuilder(ctx, slot, vIdx)
}

// getParentBlockHash retrieves the parent block hash of the block at the given slot.
// The function's behavior varies depending on the state version and whether the merge has been completed.
//
// For states of version Capella or later, the block hash is directly retrieved from the state's latest execution payload header.
//
// If the merge transition has been completed, the parent block hash is also retrieved from the state's latest execution payload header.
//
// If the activation epoch has not been reached, an errActivationNotReached error is returned.
//
// Otherwise, the terminal block hash is fetched based on the slot's time, and an error is returned if it doesn't exist.
func (vs *Server) getParentBlockHash(ctx context.Context, st state.BeaconState, slot primitives.Slot) ([]byte, error) {
	header, err := st.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, errors.Wrap(err, "could not get post capella payload header")
	}
	return header.BlockHash(), nil
}

func emptyPayloadCapella() *enginev1.ExecutionPayloadCapella {
	return &enginev1.ExecutionPayloadCapella{
		ParentHash:    make([]byte, fieldparams.RootLength),
		FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
		StateRoot:     make([]byte, fieldparams.RootLength),
		ReceiptsRoot:  make([]byte, fieldparams.RootLength),
		LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
		PrevRandao:    make([]byte, fieldparams.RootLength),
		BaseFeePerGas: make([]byte, fieldparams.RootLength),
		BlockHash:     make([]byte, fieldparams.RootLength),
		Transactions:  make([][]byte, 0),
		Withdrawals:   make([]*enginev1.Withdrawal, 0),
	}
}
