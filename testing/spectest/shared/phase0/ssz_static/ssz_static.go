package ssz_static

import (
	"context"
	"errors"
	"testing"

	fssz "github.com/prysmaticlabs/fastssz"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
	common "github.com/theQRL/qrysm/v4/testing/spectest/shared/common/ssz_static"
)

// RunSSZStaticTests executes "ssz_static" tests.
func RunSSZStaticTests(t *testing.T, config string) {
	common.RunSSZStaticTests(t, config, "phase0", unmarshalledSSZ, customHtr)
}

func customHtr(t *testing.T, htrs []common.HTR, object interface{}) []common.HTR {
	switch object.(type) {
	case *zondpb.BeaconState:
		htrs = append(htrs, func(s interface{}) ([32]byte, error) {
			beaconState, err := state_native.InitializeFromProtoPhase0(s.(*zondpb.BeaconState))
			require.NoError(t, err)
			return beaconState.HashTreeRoot(context.TODO())
		})
	}
	return htrs
}

// unmarshalledSSZ unmarshalls serialized input.
func unmarshalledSSZ(t *testing.T, serializedBytes []byte, objectName string) (interface{}, error) {
	var obj interface{}
	switch objectName {
	case "Attestation":
		obj = &zondpb.Attestation{}
	case "AttestationData":
		obj = &zondpb.AttestationData{}
	case "AttesterSlashing":
		obj = &zondpb.AttesterSlashing{}
	case "AggregateAndProof":
		obj = &zondpb.AggregateAttestationAndProof{}
	case "BeaconBlock":
		obj = &zondpb.BeaconBlock{}
	case "BeaconBlockBody":
		obj = &zondpb.BeaconBlockBody{}
	case "BeaconBlockHeader":
		obj = &zondpb.BeaconBlockHeader{}
	case "BeaconState":
		obj = &zondpb.BeaconState{}
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
	case "PendingAttestation":
		obj = &zondpb.PendingAttestation{}
	case "ProposerSlashing":
		obj = &zondpb.ProposerSlashing{}
	case "SignedAggregateAndProof":
		obj = &zondpb.SignedAggregateAttestationAndProof{}
	case "SignedBeaconBlock":
		obj = &zondpb.SignedBeaconBlock{}
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
