package validator

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/rpc/core"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	http2 "github.com/theQRL/qrysm/network/http"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

type ValidatorPerformanceRequest struct {
	PublicKeys [][]byte                    `json:"public_keys,omitempty"`
	Indices    []primitives.ValidatorIndex `json:"indices,omitempty"`
}

type ValidatorPerformanceResponse struct {
	PublicKeys                    [][]byte `json:"public_keys,omitempty"`
	CorrectlyVotedSource          []bool   `json:"correctly_voted_source,omitempty"`
	CorrectlyVotedTarget          []bool   `json:"correctly_voted_target,omitempty"`
	CorrectlyVotedHead            []bool   `json:"correctly_voted_head,omitempty"`
	CurrentEffectiveBalances      []uint64 `json:"current_effective_balances,omitempty"`
	BalancesBeforeEpochTransition []uint64 `json:"balances_before_epoch_transition,omitempty"`
	BalancesAfterEpochTransition  []uint64 `json:"balances_after_epoch_transition,omitempty"`
	MissingValidators             [][]byte `json:"missing_validators,omitempty"`
	InactivityScores              []uint64 `json:"inactivity_scores,omitempty"`
}

// GetValidatorPerformance is an HTTP handler for GetValidatorPerformance.
func (vs *Server) GetValidatorPerformance(w http.ResponseWriter, r *http.Request) {
	var req ValidatorPerformanceRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	switch {
	case errors.Is(err, io.EOF):
		handleHTTPError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		handleHTTPError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	computed, rpcError := vs.CoreService.ComputeValidatorPerformance(
		r.Context(),
		&qrysmpb.ValidatorPerformanceRequest{
			PublicKeys: req.PublicKeys,
			Indices:    req.Indices,
		},
	)
	if rpcError != nil {
		handleHTTPError(w, "Could not compute validator performance: "+rpcError.Err.Error(), core.ErrorReasonToHTTP(rpcError.Reason))
		return
	}
	response := &ValidatorPerformanceResponse{
		PublicKeys:                    computed.PublicKeys,
		CorrectlyVotedSource:          computed.CorrectlyVotedSource,
		CorrectlyVotedTarget:          computed.CorrectlyVotedTarget, // In altair, when this is true then the attestation was definitely included.
		CorrectlyVotedHead:            computed.CorrectlyVotedHead,
		CurrentEffectiveBalances:      computed.CurrentEffectiveBalances,
		BalancesBeforeEpochTransition: computed.BalancesBeforeEpochTransition,
		BalancesAfterEpochTransition:  computed.BalancesAfterEpochTransition,
		MissingValidators:             computed.MissingValidators,
		InactivityScores:              computed.InactivityScores, // Only populated in Altair
	}
	http2.WriteJson(w, response)
}

func handleHTTPError(w http.ResponseWriter, message string, code int) {
	errJson := &http2.DefaultErrorJson{
		Message: message,
		Code:    code,
	}
	http2.WriteError(w, errJson)
}
