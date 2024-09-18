package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/validator/client/beacon-api/mock"
	test_helpers "github.com/theQRL/qrysm/validator/client/beacon-api/test-helpers"
)

func TestProposeAttestation(t *testing.T) {
	attestation := &zondpb.Attestation{
		AggregationBits: test_helpers.FillByteSlice(4, 74),
		Data: &zondpb.AttestationData{
			Slot:            75,
			CommitteeIndex:  76,
			BeaconBlockRoot: test_helpers.FillByteSlice(32, 38),
			Source: &zondpb.Checkpoint{
				Epoch: 78,
				Root:  test_helpers.FillByteSlice(32, 79),
			},
			Target: &zondpb.Checkpoint{
				Epoch: 80,
				Root:  test_helpers.FillByteSlice(32, 81),
			},
		},
		Signatures: [][]byte{test_helpers.FillByteSlice(4595, 82)},
	}

	tests := []struct {
		name                 string
		attestation          *zondpb.Attestation
		expectedErrorMessage string
		endpointError        error
		endpointCall         int
	}{
		{
			name:         "valid",
			attestation:  attestation,
			endpointCall: 1,
		},
		{
			name:                 "nil attestation",
			expectedErrorMessage: "attestation is nil",
		},
		{
			name: "nil attestation data",
			attestation: &zondpb.Attestation{
				AggregationBits: test_helpers.FillByteSlice(4, 74),
				Signatures:      [][]byte{test_helpers.FillByteSlice(4595, 82)},
			},
			expectedErrorMessage: "attestation data is nil",
		},
		{
			name: "nil source checkpoint",
			attestation: &zondpb.Attestation{
				AggregationBits: test_helpers.FillByteSlice(4, 74),
				Data: &zondpb.AttestationData{
					Target: &zondpb.Checkpoint{},
				},
				Signatures: [][]byte{test_helpers.FillByteSlice(4595, 82)},
			},
			expectedErrorMessage: "source/target in attestation data is nil",
		},
		{
			name: "nil target checkpoint",
			attestation: &zondpb.Attestation{
				AggregationBits: test_helpers.FillByteSlice(4, 74),
				Data: &zondpb.AttestationData{
					Source: &zondpb.Checkpoint{},
				},
				Signatures: [][]byte{test_helpers.FillByteSlice(4595, 82)},
			},
			expectedErrorMessage: "source/target in attestation data is nil",
		},
		{
			name: "nil aggregation bits",
			attestation: &zondpb.Attestation{
				Data: &zondpb.AttestationData{
					Source: &zondpb.Checkpoint{},
					Target: &zondpb.Checkpoint{},
				},
				Signatures: [][]byte{test_helpers.FillByteSlice(4595, 82)},
			},
			expectedErrorMessage: "attestation aggregation bits is empty",
		},
		{
			name: "nil signatures",
			attestation: &zondpb.Attestation{
				AggregationBits: test_helpers.FillByteSlice(4, 74),
				Data: &zondpb.AttestationData{
					Source: &zondpb.Checkpoint{},
					Target: &zondpb.Checkpoint{},
				},
			},
			expectedErrorMessage: "attestation signatures slice is empty",
		},
		{
			name:                 "bad request",
			attestation:          attestation,
			expectedErrorMessage: "bad request",
			endpointError:        errors.New("bad request"),
			endpointCall:         1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)

			var marshalledAttestations []byte
			if checkNilAttestation(test.attestation) == nil {
				b, err := json.Marshal(jsonifyAttestations([]*zondpb.Attestation{test.attestation}))
				require.NoError(t, err)
				marshalledAttestations = b
			}

			ctx := context.Background()

			jsonRestHandler.EXPECT().PostRestJson(
				ctx,
				"/zond/v1/beacon/pool/attestations",
				nil,
				bytes.NewBuffer(marshalledAttestations),
				nil,
			).Return(
				nil,
				test.endpointError,
			).Times(test.endpointCall)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			proposeResponse, err := validatorClient.proposeAttestation(ctx, test.attestation)
			if test.expectedErrorMessage != "" {
				require.ErrorContains(t, test.expectedErrorMessage, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, proposeResponse)

			expectedAttestationDataRoot, err := attestation.Data.HashTreeRoot()
			require.NoError(t, err)

			// Make sure that the attestation data root is set
			assert.DeepEqual(t, expectedAttestationDataRoot[:], proposeResponse.AttestationDataRoot)
		})
	}
}
