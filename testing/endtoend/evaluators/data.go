package evaluators

import (
	"context"
	"errors"

	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	e2etypes "github.com/theQRL/qrysm/testing/endtoend/types"
	"google.golang.org/grpc"
)

const epochToCheck = 50 // must be more than 46 (32 hot states + 16 chkpt interval)

// ColdStateCheckpoint checks data from the database using cold state storage.
var ColdStateCheckpoint = e2etypes.Evaluator{
	Name: "cold_state_assignments_from_epoch_%d",
	Policy: func(currentEpoch primitives.Epoch) bool {
		return currentEpoch == epochToCheck
	},
	Evaluation: checkColdStateCheckpoint,
}

// Checks the first node for an old checkpoint using cold state storage.
func checkColdStateCheckpoint(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	ctx := context.Background()
	client := qrysmpb.NewBeaconChainClient(conns[0])

	for i := range primitives.Epoch(epochToCheck) {
		res, err := client.ListValidatorAssignments(ctx, &qrysmpb.ListValidatorAssignmentsRequest{
			QueryFilter: &qrysmpb.ListValidatorAssignmentsRequest_Epoch{Epoch: i},
		})
		if err != nil {
			return err
		}
		// A simple check to ensure we received some data.
		if res == nil || res.Epoch != i {
			return errors.New("failed to return a validator assignments response for an old epoch " +
				"using cold state storage from the database")
		}
	}

	return nil
}
