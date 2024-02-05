package validator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/api"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/zond/shared"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	http2 "github.com/theQRL/qrysm/v4/network/http"
	zond "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"go.opencensus.io/trace"
)

// ProduceBlockV3 Requests a beacon node to produce a valid block, which can then be signed by a validator. The
// returned block may be blinded or unblinded, depending on the current state of the network as
// decided by the execution and beacon nodes.
// The beacon node must return an unblinded block if it obtains the execution payload from its
// paired execution node. It must only return a blinded block if it obtains the execution payload
// header from an MEV relay.
// Metadata in the response indicates the type of block produced, and the supported types of block
// will be added to as forks progress.
func (s *Server) ProduceBlockV3(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.ProduceBlockV3")
	defer span.End()

	if shared.IsSyncing(r.Context(), w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}
	segments := strings.Split(r.URL.Path, "/")
	rawSlot := segments[len(segments)-1]
	rawRandaoReveal := r.URL.Query().Get("randao_reveal")
	rawGraffiti := r.URL.Query().Get("graffiti")
	rawSkipRandaoVerification := r.URL.Query().Get("skip_randao_verification")

	slot, valid := shared.ValidateUint(w, "slot", rawSlot)
	if !valid {
		return
	}

	var randaoReveal []byte
	if rawSkipRandaoVerification == "true" {
		randaoReveal = primitives.PointAtInfinity
	} else {
		rr, err := shared.DecodeHexWithLength(rawRandaoReveal, dilithium2.CryptoBytes)
		if err != nil {
			http2.HandleError(w, errors.Wrap(err, "unable to decode randao reveal").Error(), http.StatusBadRequest)
			return
		}
		randaoReveal = rr
	}
	var graffiti []byte
	if rawGraffiti != "" {
		g, err := shared.DecodeHexWithLength(rawGraffiti, 32)
		if err != nil {
			http2.HandleError(w, errors.Wrap(err, "unable to decode graffiti").Error(), http.StatusBadRequest)
			return
		}
		graffiti = g
	}

	s.produceBlockV3(ctx, w, r, &zond.BlockRequest{
		Slot:         primitives.Slot(slot),
		RandaoReveal: randaoReveal,
		Graffiti:     graffiti,
		SkipMevBoost: false,
	})
}

func (s *Server) produceBlockV3(ctx context.Context, w http.ResponseWriter, r *http.Request, v1alpha1req *zond.BlockRequest) {
	isSSZ := http2.SszRequested(r)
	if !isSSZ {
		log.Error("Checking for SSZ failed, defaulting to JSON")
	}
	v1alpha1resp, err := s.V1Alpha1Server.GetBeaconBlock(ctx, v1alpha1req)
	if err != nil {
		http2.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.ExecutionPayloadBlindedHeader, fmt.Sprintf("%v", v1alpha1resp.IsBlinded))
	w.Header().Set(api.ExecutionPayloadValueHeader, fmt.Sprintf("%d", v1alpha1resp.PayloadValue))
	phase0Block, ok := v1alpha1resp.Block.(*zond.GenericBeaconBlock_Phase0)
	if ok {
		handleProducePhase0V3(ctx, w, isSSZ, phase0Block, v1alpha1resp.PayloadValue)
		return
	}
	altairBlock, ok := v1alpha1resp.Block.(*zond.GenericBeaconBlock_Altair)
	if ok {
		handleProduceAltairV3(ctx, w, isSSZ, altairBlock, v1alpha1resp.PayloadValue)
		return
	}
	optimistic, err := s.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		http2.HandleError(w, errors.Wrap(err, "Could not determine if the node is a optimistic node").Error(), http.StatusInternalServerError)
		return
	}
	if optimistic {
		http2.HandleError(w, "The node is currently optimistic and cannot serve validators", http.StatusServiceUnavailable)
		return
	}
	blindedBellatrixBlock, ok := v1alpha1resp.Block.(*zond.GenericBeaconBlock_BlindedBellatrix)
	if ok {
		handleProduceBlindedBellatrixV3(ctx, w, isSSZ, blindedBellatrixBlock, v1alpha1resp.PayloadValue)
		return
	}
	bellatrixBlock, ok := v1alpha1resp.Block.(*zond.GenericBeaconBlock_Bellatrix)
	if ok {
		handleProduceBellatrixV3(ctx, w, isSSZ, bellatrixBlock, v1alpha1resp.PayloadValue)
		return
	}
	blindedCapellaBlock, ok := v1alpha1resp.Block.(*zond.GenericBeaconBlock_BlindedCapella)
	if ok {
		handleProduceBlindedCapellaV3(ctx, w, isSSZ, blindedCapellaBlock, v1alpha1resp.PayloadValue)
		return
	}
	capellaBlock, ok := v1alpha1resp.Block.(*zond.GenericBeaconBlock_Capella)
	if ok {
		handleProduceCapellaV3(ctx, w, isSSZ, capellaBlock, v1alpha1resp.PayloadValue)
		return
	}
}

func handleProducePhase0V3(
	ctx context.Context,
	w http.ResponseWriter,
	isSSZ bool,
	blk *zond.GenericBeaconBlock_Phase0,
	payloadValue uint64,
) {
	_, span := trace.StartSpan(ctx, "validator.ProduceBlockV3.internal.handleProducePhase0V3")
	defer span.End()
	if isSSZ {
		sszResp, err := blk.Phase0.MarshalSSZ()
		if err != nil {
			http2.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http2.WriteSsz(w, sszResp, "phase0Block.ssz")
		return
	}
	block, err := shared.BeaconBlockFromConsensus(blk.Phase0)
	if err != nil {
		http2.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(block)
	if err != nil {
		http2.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http2.WriteJson(w, &ProduceBlockV3Response{
		Version:                 version.String(version.Phase0),
		ExecutionPayloadBlinded: false,
		ExecutionPayloadValue:   fmt.Sprintf("%d", payloadValue), // mev not available at this point
		Data:                    jsonBytes,
	})
}

func handleProduceAltairV3(
	ctx context.Context,
	w http.ResponseWriter,
	isSSZ bool,
	blk *zond.GenericBeaconBlock_Altair,
	payloadValue uint64,
) {
	_, span := trace.StartSpan(ctx, "validator.ProduceBlockV3.internal.handleProduceAltairV3")
	defer span.End()
	if isSSZ {
		sszResp, err := blk.Altair.MarshalSSZ()
		if err != nil {
			http2.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http2.WriteSsz(w, sszResp, "altairBlock.ssz")
		return
	}
	block, err := shared.BeaconBlockAltairFromConsensus(blk.Altair)
	if err != nil {
		http2.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(block)
	if err != nil {
		http2.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http2.WriteJson(w, &ProduceBlockV3Response{
		Version:                 version.String(version.Altair),
		ExecutionPayloadBlinded: false,
		ExecutionPayloadValue:   fmt.Sprintf("%d", payloadValue), // mev not available at this point
		Data:                    jsonBytes,
	})
}

func handleProduceBellatrixV3(
	ctx context.Context,
	w http.ResponseWriter,
	isSSZ bool,
	blk *zond.GenericBeaconBlock_Bellatrix,
	payloadValue uint64,
) {
	_, span := trace.StartSpan(ctx, "validator.ProduceBlockV3.internal.handleProduceBellatrixV3")
	defer span.End()
	if isSSZ {
		sszResp, err := blk.Bellatrix.MarshalSSZ()
		if err != nil {
			http2.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http2.WriteSsz(w, sszResp, "bellatrixBlock.ssz")
		return
	}
	block, err := shared.BeaconBlockBellatrixFromConsensus(blk.Bellatrix)
	if err != nil {
		http2.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(block)
	if err != nil {
		http2.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http2.WriteJson(w, &ProduceBlockV3Response{
		Version:                 version.String(version.Bellatrix),
		ExecutionPayloadBlinded: false,
		ExecutionPayloadValue:   fmt.Sprintf("%d", payloadValue), // mev not available at this point
		Data:                    jsonBytes,
	})
}

func handleProduceBlindedBellatrixV3(
	ctx context.Context,
	w http.ResponseWriter,
	isSSZ bool,
	blk *zond.GenericBeaconBlock_BlindedBellatrix,
	payloadValue uint64,
) {
	_, span := trace.StartSpan(ctx, "validator.ProduceBlockV3.internal.handleProduceBlindedBellatrixV3")
	defer span.End()
	if isSSZ {
		sszResp, err := blk.BlindedBellatrix.MarshalSSZ()
		if err != nil {
			http2.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http2.WriteSsz(w, sszResp, "blindedBellatrixBlock.ssz")
		return
	}
	block, err := shared.BlindedBeaconBlockBellatrixFromConsensus(blk.BlindedBellatrix)
	if err != nil {
		http2.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(block)
	if err != nil {
		http2.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http2.WriteJson(w, &ProduceBlockV3Response{
		Version:                 version.String(version.Bellatrix),
		ExecutionPayloadBlinded: true,
		ExecutionPayloadValue:   fmt.Sprintf("%d", payloadValue),
		Data:                    jsonBytes,
	})
}

func handleProduceBlindedCapellaV3(
	ctx context.Context,
	w http.ResponseWriter,
	isSSZ bool,
	blk *zond.GenericBeaconBlock_BlindedCapella,
	payloadValue uint64,
) {
	_, span := trace.StartSpan(ctx, "validator.ProduceBlockV3.internal.handleProduceBlindedCapellaV3")
	defer span.End()
	if isSSZ {
		sszResp, err := blk.BlindedCapella.MarshalSSZ()
		if err != nil {
			http2.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http2.WriteSsz(w, sszResp, "blindedCapellaBlock.ssz")
		return
	}
	block, err := shared.BlindedBeaconBlockCapellaFromConsensus(blk.BlindedCapella)
	if err != nil {
		http2.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(block)
	if err != nil {
		http2.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http2.WriteJson(w, &ProduceBlockV3Response{
		Version:                 version.String(version.Capella),
		ExecutionPayloadBlinded: true,
		ExecutionPayloadValue:   fmt.Sprintf("%d", payloadValue),
		Data:                    jsonBytes,
	})
}

func handleProduceCapellaV3(
	ctx context.Context,
	w http.ResponseWriter,
	isSSZ bool,
	blk *zond.GenericBeaconBlock_Capella,
	payloadValue uint64,
) {
	_, span := trace.StartSpan(ctx, "validator.ProduceBlockV3.internal.handleProduceCapellaV3")
	defer span.End()
	if isSSZ {
		sszResp, err := blk.Capella.MarshalSSZ()
		if err != nil {
			http2.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http2.WriteSsz(w, sszResp, "capellaBlock.ssz")
		return
	}
	block, err := shared.BeaconBlockCapellaFromConsensus(blk.Capella)
	if err != nil {
		http2.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(block)
	if err != nil {
		http2.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http2.WriteJson(w, &ProduceBlockV3Response{
		Version:                 version.String(version.Capella),
		ExecutionPayloadBlinded: false,
		ExecutionPayloadValue:   fmt.Sprintf("%d", payloadValue), // mev not available at this point
		Data:                    jsonBytes,
	})
}
