package shared

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/theQRL/qrysm/beacon-chain/rpc/lookup"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	http2 "github.com/theQRL/qrysm/network/http"
)

// errNilValue is returned when an HTTP request payload contains a nil value
// where a non-nil object was expected.
var errNilValue = errors.New("nil value")

// DecodeError represents an error resulting from trying to decode an HTTP request.
// It tracks the full field name for which decoding failed.
type DecodeError struct {
	path []string
	err  error
}

// NewDecodeError wraps an error (either the initial decoding error or another DecodeError).
// The current field that failed decoding must be passed in.
func NewDecodeError(err error, field string) *DecodeError {
	de, ok := err.(*DecodeError)
	if ok {
		return &DecodeError{path: append([]string{field}, de.path...), err: de.err}
	}
	return &DecodeError{path: []string{field}, err: err}
}

// Error returns the formatted error message which contains the full field name and the actual decoding error.
func (e *DecodeError) Error() string {
	return fmt.Sprintf("could not decode %s: %s", strings.Join(e.path, "."), e.err.Error())
}

// IndexedVerificationFailureError wraps a collection of verification failures.
type IndexedVerificationFailureError struct {
	Message  string                        `json:"message"`
	Code     int                           `json:"code"`
	Failures []*IndexedVerificationFailure `json:"failures"`
}

func (e *IndexedVerificationFailureError) StatusCode() int {
	return e.Code
}

// IndexedVerificationFailure represents an issue when verifying a single indexed object e.g. an item in an array.
type IndexedVerificationFailure struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// WriteStateFetchError writes an appropriate error based on the supplied argument.
// The argument error should be a result of fetching state.
func WriteStateFetchError(w http.ResponseWriter, err error) {
	if errors.Is(err, stategen.ErrNoDataForSlot) {
		http2.HandleError(w, "Could not get state: lacking historical data needed to fulfill request", http.StatusNotFound)
		return
	}
	// Use errors.As so a typed error wrapped in lookup.FetchStateError (e.g.
	// the wrapping that the optimistic-status check applies before calling
	// HandleIsOptimisticError) is still recognized.
	var stateNotFoundErr *lookup.StateNotFoundError
	if errors.As(err, &stateNotFoundErr) {
		http2.HandleError(w, "Could not get state: "+stateNotFoundErr.Error(), http.StatusNotFound)
		return
	}
	var parseErr *lookup.StateIdParseError
	if errors.As(err, &parseErr) {
		http2.HandleError(w, "Invalid state ID: "+parseErr.Error(), http.StatusBadRequest)
		return
	}
	http2.HandleError(w, "Could not get state: "+err.Error(), http.StatusInternalServerError)
}

// WriteBlockFetchError writes an appropriate error based on the supplied argument.
// The argument error should be a result of fetching block.
func WriteBlockFetchError(w http.ResponseWriter, blk interfaces.ReadOnlySignedBeaconBlock, err error) bool {
	if invalidBlockIdErr, ok := err.(*lookup.BlockIdParseError); ok {
		http2.HandleError(w, "Invalid block ID: "+invalidBlockIdErr.Error(), http.StatusBadRequest)
		return false
	}
	if err != nil {
		http2.HandleError(w, "Could not get block from block ID: "+err.Error(), http.StatusInternalServerError)
		return false
	}
	if err = blocks.BeaconBlockIsNil(blk); err != nil {
		http2.HandleError(w, "Could not find requested block: "+err.Error(), http.StatusNotFound)
		return false
	}
	return true
}
