package helpers

import (
	"errors"
	"net/http"

	"github.com/theQRL/qrysm/beacon-chain/rpc/lookup"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/shared"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	http2 "github.com/theQRL/qrysm/network/http"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// HandleIsOptimisticError writes an HTTP error appropriate to the underlying
// cause of an IsOptimistic failure. State-lookup failures map to 404 / 400 via
// shared.WriteStateFetchError; missing block roots map to 404; everything
// else falls back to 500. Without this mapping a missing state would surface
// to the client as an opaque "Could not check optimistic status" 500.
func HandleIsOptimisticError(w http.ResponseWriter, err error) {
	var fetchErr *lookup.FetchStateError
	if errors.As(err, &fetchErr) {
		shared.WriteStateFetchError(w, err)
		return
	}
	var blockRootsNotFoundErr *lookup.BlockRootsNotFoundError
	if errors.As(err, &blockRootsNotFoundErr) {
		http2.HandleError(w, "Could not check optimistic status: "+err.Error(), http.StatusNotFound)
		return
	}
	http2.HandleError(w, "Could not check optimistic status: "+err.Error(), http.StatusInternalServerError)
}

// PrepareStateFetchGRPCError returns an appropriate gRPC error based on the supplied argument.
// The argument error should be a result of fetching state.
func PrepareStateFetchGRPCError(err error) error {
	if errors.Is(err, stategen.ErrNoDataForSlot) {
		return status.Errorf(codes.NotFound, "lacking historical data needed to fulfill request")
	}
	if stateNotFoundErr, ok := err.(*lookup.StateNotFoundError); ok {
		return status.Errorf(codes.NotFound, "State not found: %v", stateNotFoundErr)
	}
	if parseErr, ok := err.(*lookup.StateIdParseError); ok {
		return status.Errorf(codes.InvalidArgument, "Invalid state ID: %v", parseErr)
	}
	return status.Errorf(codes.Internal, "Invalid state ID: %v", err)
}

// IndexedVerificationFailure represents a collection of verification failures.
type IndexedVerificationFailure struct {
	Failures []*SingleIndexedVerificationFailure `json:"failures"`
}

// SingleIndexedVerificationFailure represents an issue when verifying a single indexed object e.g. an item in an array.
type SingleIndexedVerificationFailure struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

func HandleGetBlockError(blk interfaces.ReadOnlySignedBeaconBlock, err error) error {
	if invalidBlockIdErr, ok := err.(*lookup.BlockIdParseError); ok {
		return status.Errorf(codes.InvalidArgument, "Invalid block ID: %v", invalidBlockIdErr)
	}
	if err != nil {
		return status.Errorf(codes.Internal, "Could not get block from block ID: %v", err)
	}
	if err := blocks.BeaconBlockIsNil(blk); err != nil {
		return status.Errorf(codes.NotFound, "Could not find requested block: %v", err)
	}
	return nil
}
