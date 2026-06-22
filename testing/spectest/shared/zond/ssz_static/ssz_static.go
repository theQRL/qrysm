package ssz_static

import (
	"context"
	"errors"
	"testing"

	fssz "github.com/prysmaticlabs/fastssz"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	common "github.com/theQRL/qrysm/testing/spectest/shared/common/ssz_static"
)

// RunSSZStaticTests executes "ssz_static" tests.
func RunSSZStaticTests(t *testing.T, config string) {
	common.RunSSZStaticTests(t, config, "zond", unmarshalledSSZ, customHtr)
}

func customHtr(t *testing.T, htrs []common.HTR, object any) []common.HTR {
	switch object.(type) {
	case *qrysmpb.BeaconStateZond:
		htrs = append(htrs, func(s any) ([32]byte, error) {
			beaconState, err := state_native.InitializeFromProtoZond(s.(*qrysmpb.BeaconStateZond))
			require.NoError(t, err)
			return beaconState.HashTreeRoot(context.Background())
		})
	}
	return htrs
}

// unmarshalledSSZ unmarshalls serialized input.
func unmarshalledSSZ(t *testing.T, serializedBytes []byte, folderName string) (any, error) {
	var obj any
	switch folderName {
	case "ExecutionPayload":
		obj = &enginev1.ExecutionPayloadZond{}
	case "ExecutionPayloadHeader":
		obj = &enginev1.ExecutionPayloadHeaderZond{}
	case "Attestation":
		obj = &qrysmpb.Attestation{}
	case "AttestationData":
		obj = &qrysmpb.AttestationData{}
	case "AttesterSlashing":
		obj = &qrysmpb.AttesterSlashing{}
	case "AggregateAndProof":
		obj = &qrysmpb.AggregateAttestationAndProof{}
	case "BeaconBlock":
		obj = &qrysmpb.BeaconBlockZond{}
	case "BeaconBlockBody":
		obj = &qrysmpb.BeaconBlockBodyZond{}
	case "BeaconBlockHeader":
		obj = &qrysmpb.BeaconBlockHeader{}
	case "BeaconState":
		obj = &qrysmpb.BeaconStateZond{}
	case "Checkpoint":
		obj = &qrysmpb.Checkpoint{}
	case "Deposit":
		obj = &qrysmpb.Deposit{}
	case "DepositMessage":
		obj = &qrysmpb.DepositMessage{}
	case "DepositData":
		obj = &qrysmpb.Deposit_Data{}
	case "ExecutionData":
		obj = &qrysmpb.ExecutionData{}
	case "Fork":
		obj = &qrysmpb.Fork{}
	case "ForkData":
		obj = &qrysmpb.ForkData{}
	case "HistoricalBatch":
		obj = &qrysmpb.HistoricalBatch{}
	case "IndexedAttestation":
		obj = &qrysmpb.IndexedAttestation{}
	case "LightClientHeader":
		t.Skip("not a beacon node type, this is a light node type")
		return nil, nil
	case "PendingAttestation":
		obj = &qrysmpb.PendingAttestation{}
	case "ProposerSlashing":
		obj = &qrysmpb.ProposerSlashing{}
	case "SignedAggregateAndProof":
		obj = &qrysmpb.SignedAggregateAttestationAndProof{}
	case "SignedBeaconBlock":
		obj = &qrysmpb.SignedBeaconBlockZond{}
	case "SignedBeaconBlockHeader":
		obj = &qrysmpb.SignedBeaconBlockHeader{}
	case "SignedVoluntaryExit":
		obj = &qrysmpb.SignedVoluntaryExit{}
	case "SigningData":
		obj = &qrysmpb.SigningData{}
	case "Validator":
		obj = &qrysmpb.Validator{}
	case "VoluntaryExit":
		obj = &qrysmpb.VoluntaryExit{}
	case "SyncCommitteeMessage":
		obj = &qrysmpb.SyncCommitteeMessage{}
	case "SyncCommitteeContribution":
		obj = &qrysmpb.SyncCommitteeContribution{}
	case "ContributionAndProof":
		obj = &qrysmpb.ContributionAndProof{}
	case "SignedContributionAndProof":
		obj = &qrysmpb.SignedContributionAndProof{}
	case "SyncAggregate":
		obj = &qrysmpb.SyncAggregate{}
	case "SyncAggregatorSelectionData":
		obj = &qrysmpb.SyncAggregatorSelectionData{}
	case "SyncCommittee":
		obj = &qrysmpb.SyncCommittee{}
	case "HistoricalSummary":
		obj = &qrysmpb.HistoricalSummary{}
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
	// case "PowBlock":
	// 	obj = &qrysmpb.PowBlock{}
	case "Withdrawal":
		obj = &enginev1.Withdrawal{}
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
