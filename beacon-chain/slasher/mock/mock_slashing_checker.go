package mock

import (
	"context"

	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

type MockSlashingChecker struct {
	AttesterSlashingFound bool
	ProposerSlashingFound bool
	HighestAtts           map[primitives.ValidatorIndex]*zondpb.HighestAttestation
}

func (s *MockSlashingChecker) HighestAttestations(
	_ context.Context, indices []primitives.ValidatorIndex,
) ([]*zondpb.HighestAttestation, error) {
	atts := make([]*zondpb.HighestAttestation, 0, len(indices))
	for _, valIdx := range indices {
		att, ok := s.HighestAtts[valIdx]
		if !ok {
			continue
		}
		atts = append(atts, att)
	}
	return atts, nil
}

func (s *MockSlashingChecker) IsSlashableBlock(_ context.Context, _ *zondpb.SignedBeaconBlockHeader) (*zondpb.ProposerSlashing, error) {
	if s.ProposerSlashingFound {
		return &zondpb.ProposerSlashing{
			Header_1: &zondpb.SignedBeaconBlockHeader{
				Header: &zondpb.BeaconBlockHeader{
					Slot:          0,
					ProposerIndex: 0,
					ParentRoot:    params.BeaconConfig().ZeroHash[:],
					StateRoot:     params.BeaconConfig().ZeroHash[:],
					BodyRoot:      params.BeaconConfig().ZeroHash[:],
				},
				Signature: params.BeaconConfig().EmptySignature[:],
			},
			Header_2: &zondpb.SignedBeaconBlockHeader{
				Header: &zondpb.BeaconBlockHeader{
					Slot:          0,
					ProposerIndex: 0,
					ParentRoot:    params.BeaconConfig().ZeroHash[:],
					StateRoot:     params.BeaconConfig().ZeroHash[:],
					BodyRoot:      params.BeaconConfig().ZeroHash[:],
				},
				Signature: params.BeaconConfig().EmptySignature[:],
			},
		}, nil
	}
	return nil, nil
}

func (s *MockSlashingChecker) IsSlashableAttestation(_ context.Context, _ *zondpb.IndexedAttestation) ([]*zondpb.AttesterSlashing, error) {
	if s.AttesterSlashingFound {
		return []*zondpb.AttesterSlashing{
			{
				Attestation_1: &zondpb.IndexedAttestation{
					Data: &zondpb.AttestationData{},
				},
				Attestation_2: &zondpb.IndexedAttestation{
					Data: &zondpb.AttestationData{},
				},
			},
		}, nil
	}
	return nil, nil
}
