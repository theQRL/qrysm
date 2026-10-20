package helpers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/rpc/lookup"
	"github.com/theQRL/qrysm/testing/require"
)

func TestHandleIsOptimisticError(t *testing.T) {
	t.Run("fetch-state error wrapping a not-found is handled as 404", func(t *testing.T) {
		rr := httptest.NewRecorder()
		notFoundErr := lookup.NewStateNotFoundError(8)
		fetchErr := lookup.NewFetchStateError(&notFoundErr)
		HandleIsOptimisticError(rr, fetchErr)

		require.Equal(t, http.StatusNotFound, rr.Code)
		// Body should reference the underlying not-found message, not the
		// generic "Could not check optimistic status" wrapper.
		require.StringContains(t, "state not found", rr.Body.String())
	})
	t.Run("fetch-state error wrapping a state-id parse error is handled as 400", func(t *testing.T) {
		rr := httptest.NewRecorder()
		parseErr := lookup.NewStateIdParseError(errors.New("bad hex"))
		fetchErr := lookup.NewFetchStateError(&parseErr)
		HandleIsOptimisticError(rr, fetchErr)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.StringContains(t, "Invalid state ID", rr.Body.String())
	})
	t.Run("block-roots-not-found is handled as 404", func(t *testing.T) {
		rr := httptest.NewRecorder()
		HandleIsOptimisticError(rr, lookup.NewBlockRootsNotFoundError())

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.StringContains(t, "no block roots", rr.Body.String())
	})
	t.Run("generic error falls through to 500", func(t *testing.T) {
		rr := httptest.NewRecorder()
		HandleIsOptimisticError(rr, errors.New("boom"))

		require.Equal(t, http.StatusInternalServerError, rr.Code)
		require.StringContains(t, "Could not check optimistic status: boom", rr.Body.String())
	})
}
