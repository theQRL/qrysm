package ssz_static

import (
	"context"
	"errors"
	"testing"

	fssz "github.com/prysmaticlabs/fastssz"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
	common "github.com/theQRL/qrysm/v4/testing/spectest/shared/common/ssz_static"
)

// RunSSZStaticTests executes "ssz_static" tests.
func RunSSZStaticTests(t *testing.T, config string) {
	common.RunSSZStaticTests(t, config, "capella", unmarshalledSSZ, customHtr)
}

func customHtr(t *testing.T, htrs []common.HTR, object interface{}) []common.HTR {
	switch object.(type) {
	case *zondpb.BeaconStateCapella:
		htrs = append(htrs, func(s interface{}) ([32]byte, error) {
			beaconState, err := state_native.InitializeFromProtoCapella(s.(*zondpb.BeaconStateCapella))
			require.NoError(t, err)
			return beaconState.HashTreeRoot(context.Background())
		})
	}
	return htrs
}

// unmarshalledSSZ unmarshalls serialized input.
func unmarshalledSSZ(t *testing.T, serializedBytes []byte, folderName string) (interface{}, error) {
	var obj interface{}
	switch folderName {
	case "ExecutionPayload":
		obj = &enginev1.ExecutionPayloadCapella{}
	case "ExecutionPayloadHeader":
		obj = &enginev1.ExecutionPayloadHeaderCapella{}
	case "Attestation":
		obj = &zondpb.Attestation{}
	case "AttestationData":
		obj = &zondpb.AttestationData{}
	case "AttesterSlashing":
		obj = &zondpb.AttesterSlashing{}
	case "AggregateAndProof":
		obj = &zondpb.AggregateAttestationAndProof{}
	case "BeaconBlock":
		obj = &zondpb.BeaconBlockCapella{}
	case "BeaconBlockBody":
		obj = &zondpb.BeaconBlockBodyCapella{}
	case "BeaconBlockHeader":
		obj = &zondpb.BeaconBlockHeader{}
	case "BeaconState":
		obj = &zondpb.BeaconStateCapella{}
	case "Checkpoint":
		obj = &zondpb.Checkpoint{}
	case "Deposit":
		obj = &zondpb.Deposit{}
	case "DepositMessage":
		obj = &zondpb.DepositMessage{}
	case "DepositData":
		obj = &zondpb.Deposit_Data{}
	case "Eth1Data":
		obj = &zondpb.Eth1Data{}
	case "Eth1Block":
		t.Skip("Unused type")
		return nil, nil
	case "Fork":
		obj = &zondpb.Fork{}
	case "ForkData":
		obj = &zondpb.ForkData{}
	case "HistoricalBatch":
		obj = &zondpb.HistoricalBatch{}
	case "IndexedAttestation":
		obj = &zondpb.IndexedAttestation{}
	case "LightClientHeader":
		t.Skip("not a beacon node type, this is a light node type")
		return nil, nil
	case "PendingAttestation":
		obj = &zondpb.PendingAttestation{}
	case "ProposerSlashing":
		obj = &zondpb.ProposerSlashing{}
	case "SignedAggregateAndProof":
		obj = &zondpb.SignedAggregateAttestationAndProof{}
	case "SignedBeaconBlock":
		obj = &zondpb.SignedBeaconBlockCapella{}
	case "SignedBeaconBlockHeader":
		obj = &zondpb.SignedBeaconBlockHeader{}
	case "SignedVoluntaryExit":
		obj = &zondpb.SignedVoluntaryExit{}
	case "SigningData":
		obj = &zondpb.SigningData{}
	case "Validator":
		obj = &zondpb.Validator{}
	case "VoluntaryExit":
		obj = &zondpb.VoluntaryExit{}
	case "SyncCommitteeMessage":
		obj = &zondpb.SyncCommitteeMessage{}
	case "SyncCommitteeContribution":
		obj = &zondpb.SyncCommitteeContribution{}
	case "ContributionAndProof":
		obj = &zondpb.ContributionAndProof{}
	case "SignedContributionAndProof":
		obj = &zondpb.SignedContributionAndProof{}
	case "SyncAggregate":
		obj = &zondpb.SyncAggregate{}
	case "SyncAggregatorSelectionData":
		obj = &zondpb.SyncAggregatorSelectionData{}
	case "SyncCommittee":
		obj = &zondpb.SyncCommittee{}
	case "HistoricalSummary":
		obj = &zondpb.HistoricalSummary{}
	case "LightClientOptimisticUpdate":
		t.Skip("not a beacon node type, this is a light node type")
		return nil, nil
	case "LightClientFinalityUpdate":
		t.Skip("not a beacon node type, this is a light node type")
		return nil, nil
	case "LightClientBootstrap":
		t.Skip("not a beacon node type, this is a light node type")
		return nil, nil
	case "LightClientSnapshot":
		t.Skip("not a beacon node type, this is a light node type")
		return nil, nil
	case "LightClientUpdate":
		t.Skip("not a beacon node type, this is a light node type")
		return nil, nil
	case "PowBlock":
		obj = &zondpb.PowBlock{}
	case "Withdrawal":
		obj = &enginev1.Withdrawal{}
	case "DilithiumToExecutionChange":
		obj = &zondpb.DilithiumToExecutionChange{}
	case "SignedDilithiumToExecutionChange":
		obj = &zondpb.SignedDilithiumToExecutionChange{}
	default:
		return nil, errors.New("type not found")
	}
	var err error
	if o, ok := obj.(fssz.Unmarshaler); ok {
		err = o.UnmarshalSSZ(serializedBytes)
	} else {
		err = errors.New("could not unmarshal object, not a fastssz compatible object")
	}
	return obj, err
}
