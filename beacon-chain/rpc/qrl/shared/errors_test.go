package shared

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	pkgerrors "github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/rpc/lookup"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	http2 "github.com/theQRL/qrysm/network/http"
	"github.com/theQRL/qrysm/testing/assert"
)

func TestDecodeError(t *testing.T) {
	e := pkgerrors.New("not a number")
	de := NewDecodeError(e, "Z")
	de = NewDecodeError(de, "Y")
	de = NewDecodeError(de, "X")
	assert.Equal(t, "could not decode X.Y.Z: not a number", de.Error())
}

func TestWriteStateFetchError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		code        int
		wantMessage string
	}{
		{
			name:        "no data for slot returns not found",
			err:         pkgerrors.Wrap(stategen.ErrNoDataForSlot, "slot 123 not in db due to checkpoint sync"),
			code:        http.StatusNotFound,
			wantMessage: "Could not get state: lacking historical data needed to fulfill request",
		},
		{
			name: "state not found returns not found",
			err: func() error {
				err := lookup.NewStateNotFoundError(128)
				return &err
			}(),
			code:        http.StatusNotFound,
			wantMessage: "Could not get state: state not found in the last 128 state roots",
		},
		{
			name: "state id parse error returns bad request",
			err: func() error {
				err := lookup.NewStateIdParseError(errors.New("bad id"))
				return &err
			}(),
			code:        http.StatusBadRequest,
			wantMessage: "Invalid state ID: could not parse state ID: bad id",
		},
		{
			name:        "generic error returns internal server error",
			err:         errors.New("boom"),
			code:        http.StatusInternalServerError,
			wantMessage: "Could not get state: boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			WriteStateFetchError(recorder, tt.err)

			assert.Equal(t, tt.code, recorder.Code)
			resp := &http2.DefaultErrorJson{}
			if err := json.Unmarshal(recorder.Body.Bytes(), resp); err != nil {
				t.Fatalf("failed to decode error response: %v", err)
			}
			assert.Equal(t, tt.code, resp.Code)
			assert.StringContains(t, tt.wantMessage, resp.Message)
		})
	}
}
