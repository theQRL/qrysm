package beacon_api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/apimiddleware"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/validator/client/beacon-api/mock"
	test_helpers "github.com/theQRL/qrysm/v4/validator/client/beacon-api/test-helpers"
)

func TestGetBeaconBlock_RequestFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetRestJsonResponse(
		ctx,
		gomock.Any(),
		gomock.Any(),
	).Return(
		nil,
		errors.New("foo error"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	_, err := validatorClient.getBeaconBlock(ctx, 1, []byte{1}, []byte{2})
	assert.ErrorContains(t, "failed to query GET REST endpoint", err)
	assert.ErrorContains(t, "foo error", err)
}

func TestGetBeaconBlock_Error(t *testing.T) {
	capellaBeaconBlockBytes, err := json.Marshal(apimiddleware.BeaconBlockCapellaJson{})
	require.NoError(t, err)

	testCases := []struct {
		name                 string
		beaconBlock          interface{}
		expectedErrorMessage string
		consensusVersion     string
		data                 json.RawMessage
	}{
		{
			name:                 "capella block decoding failed",
			expectedErrorMessage: "failed to decode capella block response json",
			beaconBlock:          "foo",
			consensusVersion:     "capella",
			data:                 []byte{},
		},
		{
			name:                 "capella block conversion failed",
			expectedErrorMessage: "failed to get capella block",
			consensusVersion:     "capella",
			data:                 capellaBeaconBlockBytes,
		},
		{
			name:                 "unsupported consensus version",
			expectedErrorMessage: "unsupported consensus version `foo`",
			consensusVersion:     "foo",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()

			jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
			jsonRestHandler.EXPECT().GetRestJsonResponse(
				ctx,
				gomock.Any(),
				&abstractProduceBlockResponseJson{},
			).SetArg(
				2,
				abstractProduceBlockResponseJson{
					Version: testCase.consensusVersion,
					Data:    testCase.data,
				},
			).Return(
				nil,
				nil,
			).Times(1)

			beaconBlockConverter := mock.NewMockbeaconBlockConverter(ctrl)
			beaconBlockConverter.EXPECT().ConvertRESTCapellaBlockToProto(
				gomock.Any(),
			).Return(
				nil,
				errors.New(testCase.expectedErrorMessage),
			).AnyTimes()

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler, beaconBlockConverter: beaconBlockConverter}
			_, err := validatorClient.getBeaconBlock(ctx, 1, []byte{1}, []byte{2})
			assert.ErrorContains(t, testCase.expectedErrorMessage, err)
		})
	}
}

func TestGetBeaconBlock_CapellaValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	capellaProtoBeaconBlock := test_helpers.GenerateProtoCapellaBeaconBlock()
	capellaBeaconBlock := test_helpers.GenerateJsonCapellaBeaconBlock()
	capellaBeaconBlockBytes, err := json.Marshal(capellaBeaconBlock)
	require.NoError(t, err)

	const slot = primitives.Slot(1)
	randaoReveal := []byte{2}
	graffiti := []byte{3}

	ctx := context.Background()

	jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetRestJsonResponse(
		ctx,
		fmt.Sprintf("/zond/v1/validator/blocks/%d?graffiti=%s&randao_reveal=%s", slot, hexutil.Encode(graffiti), hexutil.Encode(randaoReveal)),
		&abstractProduceBlockResponseJson{},
	).SetArg(
		2,
		abstractProduceBlockResponseJson{
			Version: "capella",
			Data:    capellaBeaconBlockBytes,
		},
	).Return(
		nil,
		nil,
	).Times(1)

	beaconBlockConverter := mock.NewMockbeaconBlockConverter(ctrl)
	beaconBlockConverter.EXPECT().ConvertRESTCapellaBlockToProto(
		capellaBeaconBlock,
	).Return(
		capellaProtoBeaconBlock,
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler, beaconBlockConverter: beaconBlockConverter}
	beaconBlock, err := validatorClient.getBeaconBlock(ctx, slot, randaoReveal, graffiti)
	require.NoError(t, err)

	expectedBeaconBlock := &zondpb.GenericBeaconBlock{
		Block: &zondpb.GenericBeaconBlock_Capella{
			Capella: capellaProtoBeaconBlock,
		},
	}

	assert.DeepEqual(t, expectedBeaconBlock, beaconBlock)
}
