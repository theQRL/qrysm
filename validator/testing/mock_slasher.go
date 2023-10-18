package testing

import (
	"context"
	"errors"

	zond "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// MockSlasher mocks the slasher rpc server.
type MockSlasher struct {
	SlashAttestation             bool
	SlashBlock                   bool
	IsSlashableAttestationCalled bool
	IsSlashableBlockCalled       bool
}

// HighestAttestations will return an empty array of attestations.
func (MockSlasher) HighestAttestations(_ context.Context, _ *zond.HighestAttestationRequest, _ ...grpc.CallOption) (*zond.HighestAttestationResponse, error) {
	return &zond.HighestAttestationResponse{
		Attestations: nil,
	}, nil
}

// IsSlashableAttestation returns slashbale attestation if slash attestation is set to true.
func (ms MockSlasher) IsSlashableAttestation(_ context.Context, in *zond.IndexedAttestation, _ ...grpc.CallOption) (*zond.AttesterSlashingResponse, error) {
	ms.IsSlashableAttestationCalled = true // skipcq: RVV-B0006
	if ms.SlashAttestation {
		slashingAtt, ok := proto.Clone(in).(*zond.IndexedAttestation)
		if !ok {
			return nil, errors.New("object is not of type *zond.IndexedAttestation")
		}
		slashingAtt.Data.BeaconBlockRoot = []byte("slashing")
		slashings := []*zond.AttesterSlashing{{
			Attestation_1: in,
			Attestation_2: slashingAtt,
		},
		}
		return &zond.AttesterSlashingResponse{
			AttesterSlashings: slashings,
		}, nil
	}
	return nil, nil
}

// IsSlashableBlock returns proposer slashing if slash block is set to true.
func (ms MockSlasher) IsSlashableBlock(_ context.Context, in *zond.SignedBeaconBlockHeader, _ ...grpc.CallOption) (*zond.ProposerSlashingResponse, error) {
	ms.IsSlashableBlockCalled = true // skipcq: RVV-B0006
	if ms.SlashBlock {
		slashingBlk, ok := proto.Clone(in).(*zond.SignedBeaconBlockHeader)
		if !ok {
			return nil, errors.New("object is not of type *zond.SignedBeaconBlockHeader")
		}
		slashingBlk.Header.BodyRoot = []byte("slashing")
		slashings := []*zond.ProposerSlashing{{
			Header_1: in,
			Header_2: slashingBlk,
		},
		}
		return &zond.ProposerSlashingResponse{
			ProposerSlashings: slashings,
		}, nil
	}
	return nil, nil
}
