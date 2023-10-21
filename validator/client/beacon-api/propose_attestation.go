package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

func (c beaconApiValidatorClient) proposeAttestation(ctx context.Context, attestation *zondpb.Attestation) (*zondpb.AttestResponse, error) {
	if err := checkNilAttestation(attestation); err != nil {
		return nil, err
	}

	marshalledAttestation, err := json.Marshal(jsonifyAttestations([]*zondpb.Attestation{attestation}))
	if err != nil {
		return nil, err
	}

	if _, err := c.jsonRestHandler.PostRestJson(ctx, "/zond/v1/beacon/pool/attestations", nil, bytes.NewBuffer(marshalledAttestation), nil); err != nil {
		return nil, errors.Wrap(err, "failed to send POST data to REST endpoint")
	}

	attestationDataRoot, err := attestation.Data.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute attestation data root")
	}

	return &zondpb.AttestResponse{AttestationDataRoot: attestationDataRoot[:]}, nil
}

// checkNilAttestation returns error if attestation or any field of attestation is nil.
func checkNilAttestation(attestation *zondpb.Attestation) error {
	if attestation == nil {
		return errors.New("attestation is nil")
	}

	if attestation.Data == nil {
		return errors.New("attestation data is nil")
	}

	if attestation.Data.Source == nil || attestation.Data.Target == nil {
		return errors.New("source/target in attestation data is nil")
	}

	if len(attestation.AggregationBits) == 0 {
		return errors.New("attestation aggregation bits is empty")
	}

	if len(attestation.Signature) == 0 {
		return errors.New("attestation signature is empty")
	}

	return nil
}
