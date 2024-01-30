package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/apimiddleware"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/validator/client/beacon-api/mock"
)

const submitSignedContributionAndProofTestEndpoint = "/zond/v1/validator/contribution_and_proofs"

func TestSubmitSignedContributionAndProof_Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jsonContributionAndProofs := []apimiddleware.SignedContributionAndProofJson{
		{
			Message: &apimiddleware.ContributionAndProofJson{
				AggregatorIndex: "1",
				Contribution: &apimiddleware.SyncCommitteeContributionJson{
					Slot:              "2",
					BeaconBlockRoot:   hexutil.Encode([]byte{3}),
					SubcommitteeIndex: "4",
					AggregationBits:   hexutil.Encode([]byte{5}),
					Signature:         hexutil.Encode([]byte{6}),
				},
				SelectionProof: hexutil.Encode([]byte{7}),
			},
			Signature: hexutil.Encode([]byte{8}),
		},
	}

	marshalledContributionAndProofs, err := json.Marshal(jsonContributionAndProofs)
	require.NoError(t, err)

	ctx := context.Background()

	jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().PostRestJson(
		ctx,
		submitSignedContributionAndProofTestEndpoint,
		nil,
		bytes.NewBuffer(marshalledContributionAndProofs),
		nil,
	).Return(
		nil,
		nil,
	).Times(1)

	contributionAndProof := &zondpb.SignedContributionAndProof{
		Message: &zondpb.ContributionAndProof{
			AggregatorIndex: 1,
			Contribution: &zondpb.SyncCommitteeContribution{
				Slot:              2,
				BlockRoot:         []byte{3},
				SubcommitteeIndex: 4,
				AggregationBits:   []byte{5},
				Signature:         []byte{6},
			},
			SelectionProof: []byte{7},
		},
		Signature: []byte{8},
	}

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	err = validatorClient.submitSignedContributionAndProof(ctx, contributionAndProof)
	require.NoError(t, err)
}

func TestSubmitSignedContributionAndProof_Error(t *testing.T) {
	testCases := []struct {
		name                 string
		data                 *zondpb.SignedContributionAndProof
		expectedErrorMessage string
		httpRequestExpected  bool
	}{
		{
			name:                 "nil signed contribution and proof",
			data:                 nil,
			expectedErrorMessage: "signed contribution and proof is nil",
		},
		{
			name:                 "nil message",
			data:                 &zondpb.SignedContributionAndProof{},
			expectedErrorMessage: "signed contribution and proof message is nil",
		},
		{
			name: "nil contribution",
			data: &zondpb.SignedContributionAndProof{
				Message: &zondpb.ContributionAndProof{},
			},
			expectedErrorMessage: "signed contribution and proof contribution is nil",
		},
		{
			name: "bad request",
			data: &zondpb.SignedContributionAndProof{
				Message: &zondpb.ContributionAndProof{
					Contribution: &zondpb.SyncCommitteeContribution{},
				},
			},
			httpRequestExpected:  true,
			expectedErrorMessage: "failed to send POST data to REST endpoint: foo error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()

			jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
			if testCase.httpRequestExpected {
				jsonRestHandler.EXPECT().PostRestJson(
					ctx,
					submitSignedContributionAndProofTestEndpoint,
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(
					nil,
					errors.New("foo error"),
				).Times(1)
			}

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			err := validatorClient.submitSignedContributionAndProof(ctx, testCase.data)
			assert.ErrorContains(t, testCase.expectedErrorMessage, err)
		})
	}
}
