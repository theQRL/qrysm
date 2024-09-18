package beacon_api

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	zond "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/validator/client/beacon-api/mock"
	test_helpers "github.com/theQRL/qrysm/validator/client/beacon-api/test-helpers"
)

func TestStreamBlocks_UnsupportedConsensusVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().GetRestJsonResponse(
		ctx,
		gomock.Any(),
		&abstractSignedBlockResponseJson{},
	).SetArg(
		2,
		abstractSignedBlockResponseJson{Version: "foo"},
	).Return(
		nil,
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	streamBlocksClient := validatorClient.streamBlocks(ctx, &zond.StreamBlocksRequest{}, time.Millisecond*100)
	_, err := streamBlocksClient.Recv()
	assert.ErrorContains(t, "unsupported consensus version `foo`", err)
}

func TestStreamBlocks_Error(t *testing.T) {
	testSuites := []struct {
		consensusVersion             string
		generateBeaconBlockConverter func(ctrl *gomock.Controller, conversionError error) *mock.MockbeaconBlockConverter
	}{
		{
			consensusVersion: "capella",
			generateBeaconBlockConverter: func(ctrl *gomock.Controller, conversionError error) *mock.MockbeaconBlockConverter {
				beaconBlockConverter := mock.NewMockbeaconBlockConverter(ctrl)
				beaconBlockConverter.EXPECT().ConvertRESTCapellaBlockToProto(
					gomock.Any(),
				).Return(
					nil,
					conversionError,
				).AnyTimes()

				return beaconBlockConverter
			},
		},
	}

	testCases := []struct {
		name                 string
		expectedErrorMessage string
		conversionError      error
		generateData         func(consensusVersion string) []byte
	}{
		{
			name:                 "block decoding failed",
			expectedErrorMessage: "failed to decode signed %s block response json",
			generateData:         func(consensusVersion string) []byte { return []byte{} },
		},
		{
			name:                 "block conversion failed",
			expectedErrorMessage: "failed to get signed %s block",
			conversionError:      errors.New("foo"),
			generateData: func(consensusVersion string) []byte {
				blockBytes, err := json.Marshal(apimiddleware.SignedBeaconBlockJson{Signature: "0x01"})
				require.NoError(t, err)
				return blockBytes
			},
		},
		{
			name:                 "signature decoding failed",
			expectedErrorMessage: "failed to decode %s block signature `foo`",
			generateData: func(consensusVersion string) []byte {
				blockBytes, err := json.Marshal(apimiddleware.SignedBeaconBlockJson{Signature: "foo"})
				require.NoError(t, err)
				return blockBytes
			},
		},
	}

	for _, testSuite := range testSuites {
		t.Run(testSuite.consensusVersion, func(t *testing.T) {
			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					ctrl := gomock.NewController(t)
					defer ctrl.Finish()

					ctx := context.Background()

					jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
					jsonRestHandler.EXPECT().GetRestJsonResponse(
						ctx,
						gomock.Any(),
						&abstractSignedBlockResponseJson{},
					).SetArg(
						2,
						abstractSignedBlockResponseJson{
							Version: testSuite.consensusVersion,
							Data:    testCase.generateData(testSuite.consensusVersion),
						},
					).Return(
						nil,
						nil,
					).Times(1)

					beaconBlockConverter := testSuite.generateBeaconBlockConverter(ctrl, testCase.conversionError)
					validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler, beaconBlockConverter: beaconBlockConverter}
					streamBlocksClient := validatorClient.streamBlocks(ctx, &zond.StreamBlocksRequest{}, time.Millisecond*100)

					_, err := streamBlocksClient.Recv()
					assert.ErrorContains(t, fmt.Sprintf(testCase.expectedErrorMessage, testSuite.consensusVersion), err)
				})
			}
		})
	}

}

func TestStreamBlocks_CapellaValid(t *testing.T) {
	testCases := []struct {
		name         string
		verifiedOnly bool
	}{
		{
			name:         "verified optional",
			verifiedOnly: false,
		},
		{
			name:         "verified only",
			verifiedOnly: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()

			signedBlockResponseJson := abstractSignedBlockResponseJson{}
			jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
			beaconBlockConverter := mock.NewMockbeaconBlockConverter(ctrl)

			// For the first call, return a block that satisfies the verifiedOnly condition. This block should be returned by the first Recv().
			// For the second call, return the same block as the previous one. This block shouldn't be returned by the second Recv().
			capellaBeaconBlock1 := test_helpers.GenerateJsonCapellaBeaconBlock()
			capellaBeaconBlock1.Slot = "1"
			signedBeaconBlockContainer1 := apimiddleware.SignedBeaconBlockCapellaJson{
				Message:   capellaBeaconBlock1,
				Signature: "0x01",
			}

			marshalledSignedBeaconBlockContainer1, err := json.Marshal(signedBeaconBlockContainer1)
			require.NoError(t, err)

			jsonRestHandler.EXPECT().GetRestJsonResponse(
				ctx,
				"/zond/v1/beacon/blocks/head",
				&signedBlockResponseJson,
			).Return(
				nil,
				nil,
			).SetArg(
				2,
				abstractSignedBlockResponseJson{
					Version:             "capella",
					ExecutionOptimistic: false,
					Data:                marshalledSignedBeaconBlockContainer1,
				},
			).Times(2)

			capellaProtoBeaconBlock1 := test_helpers.GenerateProtoCapellaBeaconBlock()
			capellaProtoBeaconBlock1.Slot = 1

			beaconBlockConverter.EXPECT().ConvertRESTCapellaBlockToProto(
				capellaBeaconBlock1,
			).Return(
				capellaProtoBeaconBlock1,
				nil,
			).Times(2)

			// For the third call, return a block with a different slot than the previous one, but with the verifiedOnly condition not satisfied.
			// If verifiedOnly == false, this block will be returned by the second Recv(); otherwise, another block will be requested.
			capellaBeaconBlock2 := test_helpers.GenerateJsonCapellaBeaconBlock()
			capellaBeaconBlock2.Slot = "2"
			signedBeaconBlockContainer2 := apimiddleware.SignedBeaconBlockCapellaJson{
				Message:   capellaBeaconBlock2,
				Signature: "0x02",
			}

			marshalledSignedBeaconBlockContainer2, err := json.Marshal(signedBeaconBlockContainer2)
			require.NoError(t, err)

			jsonRestHandler.EXPECT().GetRestJsonResponse(
				ctx,
				"/zond/v1/beacon/blocks/head",
				&signedBlockResponseJson,
			).Return(
				nil,
				nil,
			).SetArg(
				2,
				abstractSignedBlockResponseJson{
					Version:             "capella",
					ExecutionOptimistic: true,
					Data:                marshalledSignedBeaconBlockContainer2,
				},
			).Times(1)

			capellaProtoBeaconBlock2 := test_helpers.GenerateProtoCapellaBeaconBlock()
			capellaProtoBeaconBlock2.Slot = 2

			beaconBlockConverter.EXPECT().ConvertRESTCapellaBlockToProto(
				capellaBeaconBlock2,
			).Return(
				capellaProtoBeaconBlock2,
				nil,
			).Times(1)

			// The fourth call is only necessary when verifiedOnly == true since the previous block was optimistic
			if testCase.verifiedOnly {
				jsonRestHandler.EXPECT().GetRestJsonResponse(
					ctx,
					"/zond/v1/beacon/blocks/head",
					&signedBlockResponseJson,
				).Return(
					nil,
					nil,
				).SetArg(
					2,
					abstractSignedBlockResponseJson{
						Version:             "capella",
						ExecutionOptimistic: false,
						Data:                marshalledSignedBeaconBlockContainer2,
					},
				).Times(1)

				beaconBlockConverter.EXPECT().ConvertRESTCapellaBlockToProto(
					capellaBeaconBlock2,
				).Return(
					capellaProtoBeaconBlock2,
					nil,
				).Times(1)
			}

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler, beaconBlockConverter: beaconBlockConverter}
			streamBlocksClient := validatorClient.streamBlocks(ctx, &zond.StreamBlocksRequest{VerifiedOnly: testCase.verifiedOnly}, time.Millisecond*100)

			// Get the first block
			streamBlocksResponse1, err := streamBlocksClient.Recv()
			require.NoError(t, err)

			expectedStreamBlocksResponse1 := &zond.StreamBlocksResponse{
				Block: &zond.StreamBlocksResponse_CapellaBlock{
					CapellaBlock: &zond.SignedBeaconBlockCapella{
						Block:     capellaProtoBeaconBlock1,
						Signature: []byte{1},
					},
				},
			}

			assert.DeepEqual(t, expectedStreamBlocksResponse1, streamBlocksResponse1)

			// Get the second block
			streamBlocksResponse2, err := streamBlocksClient.Recv()
			require.NoError(t, err)

			expectedStreamBlocksResponse2 := &zond.StreamBlocksResponse{
				Block: &zond.StreamBlocksResponse_CapellaBlock{
					CapellaBlock: &zond.SignedBeaconBlockCapella{
						Block:     capellaProtoBeaconBlock2,
						Signature: []byte{2},
					},
				},
			}

			assert.DeepEqual(t, expectedStreamBlocksResponse2, streamBlocksResponse2)
		})
	}
}
