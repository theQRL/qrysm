package shared_test

import (
	"encoding/json"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/shared"
	rpctesting "github.com/theQRL/qrysm/beacon-chain/rpc/qrl/shared/testing"
	"github.com/theQRL/qrysm/testing/require"
)

func TestDepositsToConsensus_NilElement(t *testing.T) {
	_, err := shared.DepositsToConsensus([]*shared.Deposit{nil})
	require.ErrorContains(t, "[0]", err)
}

// A published JSON block with a `null` deposit must be refused by the
// conversion, like every other null operation element, not panic in the handler.
func TestSignedBeaconBlockZond_ToGeneric_NullDepositElement(t *testing.T) {
	var b shared.SignedBeaconBlockZond
	require.NoError(t, json.Unmarshal([]byte(rpctesting.ZondBlock), &b))
	b.Message.Body.Deposits = []*shared.Deposit{nil}
	_, err := b.ToGeneric()
	require.ErrorContains(t, "Deposits.[0]", err)
}
