package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/validator"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/validator/client/beacon-api/mock"
)

func TestDutyDependentRootsProvider_GetDutyDependentRoots(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	epoch := primitives.Epoch(3)
	emptyValidatorIndices, err := json.Marshal([]string{})
	require.NoError(t, err)

	previousRoot := []byte{1, 2, 3}
	currentRoot := []byte{4, 5, 6}
	jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().PostRestJson(
		ctx,
		fmt.Sprintf("/qrl/v1/validator/duties/attester/%d", epoch),
		nil,
		bytes.NewBuffer(emptyValidatorIndices),
		&validator.GetAttesterDutiesResponse{},
	).Return(
		nil,
		nil,
	).SetArg(
		4,
		validator.GetAttesterDutiesResponse{DependentRoot: hexutil.Encode(previousRoot)},
	).Times(1)
	jsonRestHandler.EXPECT().GetRestJsonResponse(
		ctx,
		fmt.Sprintf("/qrl/v1/validator/duties/proposer/%d", epoch),
		&validator.GetProposerDutiesResponse{},
	).Return(
		nil,
		nil,
	).SetArg(
		2,
		validator.GetProposerDutiesResponse{DependentRoot: hexutil.Encode(currentRoot)},
	).Times(1)

	provider := &DutyDependentRootsProvider{jsonRestHandler: jsonRestHandler}
	gotPreviousRoot, gotCurrentRoot, err := provider.GetDutyDependentRoots(ctx, epoch)
	require.NoError(t, err)
	assert.DeepEqual(t, previousRoot, gotPreviousRoot)
	assert.DeepEqual(t, currentRoot, gotCurrentRoot)
}
