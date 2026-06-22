package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/validator/client/beacon-api/mock"
	test_helpers "github.com/theQRL/qrysm/validator/client/beacon-api/test-helpers"
)

func TestSubmitSignedAggregateSelectionProof_Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProof := generateSignedAggregateAndProofJson()
	marshalledSignedAggregateSignedAndProof, err := json.Marshal([]*apimiddleware.SignedAggregateAttestationAndProofJson{jsonifySignedAggregateAndProof(signedAggregateAndProof)})
	require.NoError(t, err)

	ctx := context.Background()

	jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().PostRestJson(
		ctx,
		"/qrl/v1/validator/aggregate_and_proofs",
		nil,
		bytes.NewBuffer(marshalledSignedAggregateSignedAndProof),
		nil,
	).Return(
		nil,
		nil,
	).Times(1)

	attestationDataRoot, err := signedAggregateAndProof.Message.Aggregate.Data.HashTreeRoot()
	require.NoError(t, err)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	resp, err := validatorClient.submitSignedAggregateSelectionProof(ctx, &qrysmpb.SignedAggregateSubmitRequest{
		SignedAggregateAndProof: signedAggregateAndProof,
	})
	require.NoError(t, err)
	assert.DeepEqual(t, attestationDataRoot[:], resp.AttestationDataRoot)
}

func TestSubmitSignedAggregateSelectionProof_BadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProof := generateSignedAggregateAndProofJson()
	marshalledSignedAggregateSignedAndProof, err := json.Marshal([]*apimiddleware.SignedAggregateAttestationAndProofJson{jsonifySignedAggregateAndProof(signedAggregateAndProof)})
	require.NoError(t, err)

	ctx := context.Background()
	jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().PostRestJson(
		ctx,
		"/qrl/v1/validator/aggregate_and_proofs",
		nil,
		bytes.NewBuffer(marshalledSignedAggregateSignedAndProof),
		nil,
	).Return(
		nil,
		errors.New("bad request"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	_, err = validatorClient.submitSignedAggregateSelectionProof(ctx, &qrysmpb.SignedAggregateSubmitRequest{
		SignedAggregateAndProof: signedAggregateAndProof,
	})
	assert.ErrorContains(t, "failed to send POST data to REST endpoint", err)
	assert.ErrorContains(t, "bad request", err)
}

func generateSignedAggregateAndProofJson() *qrysmpb.SignedAggregateAttestationAndProof {
	return &qrysmpb.SignedAggregateAttestationAndProof{
		Message: &qrysmpb.AggregateAttestationAndProof{
			AggregatorIndex: 72,
			Aggregate: &qrysmpb.Attestation{
				AggregationBits: test_helpers.FillByteSlice(4, 74),
				Data: &qrysmpb.AttestationData{
					Slot:            75,
					CommitteeIndex:  76,
					BeaconBlockRoot: test_helpers.FillByteSlice(32, 38),
					Source: &qrysmpb.Checkpoint{
						Epoch: 78,
						Root:  test_helpers.FillByteSlice(32, 79),
					},
					Target: &qrysmpb.Checkpoint{
						Epoch: 80,
						Root:  test_helpers.FillByteSlice(32, 81),
					},
				},
				Signatures: [][]byte{test_helpers.FillByteSlice(4627, 82)},
			},
			SelectionProof: test_helpers.FillByteSlice(4627, 82),
		},
		Signature: test_helpers.FillByteSlice(4627, 82),
	}
}
