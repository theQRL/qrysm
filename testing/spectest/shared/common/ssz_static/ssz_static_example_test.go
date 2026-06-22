package ssz_static_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	fssz "github.com/prysmaticlabs/fastssz"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	common "github.com/theQRL/qrysm/testing/spectest/shared/common/ssz_static"
)

func ExampleRunSSZStaticTests() {
	// Define an unmarshaller to select the correct go type based on the string
	// name provided in spectests and then populate it with the serialized bytes.
	unmarshaller := func(t *testing.T, serializedBytes []byte, objectName string) (any, error) {
		var obj any
		switch objectName {
		case "Attestation":
			obj = &qrysmpb.Attestation{}
		case "BeaconState":
			obj = &qrysmpb.BeaconStateZond{}
		default:
			return nil, fmt.Errorf("unsupported type: %s", objectName)
		}
		var err error
		if o, ok := obj.(fssz.Unmarshaler); ok {
			err = o.UnmarshalSSZ(serializedBytes)
		} else {
			err = errors.New("could not unmarshal object, not a fastssz compatible object")
		}
		return obj, err
	}

	// Optional: define a method to add custom HTR methods for a given object.
	// This argument may be nil if your test does not require custom HTR methods.
	// Most commonly, this is used when a handwritten HTR method with specialized caching
	// is used and you want to ensure it passes spectests.
	customHTR := func(t *testing.T, htrs []common.HTR, object any) []common.HTR {
		switch object.(type) {
		case *qrysmpb.BeaconBlockBodyZond:
			htrs = append(htrs, func(s any) ([32]byte, error) {
				beaconState, err := state_native.InitializeFromProtoZond(s.(*qrysmpb.BeaconStateZond))
				require.NoError(t, err)
				return beaconState.HashTreeRoot(context.TODO())
			})
		}
		return htrs
	}

	var t *testing.T
	// common.RunSSZStaticTests will run all of the tests found in the spec test folder with the
	// given config and forkOrPhase. It will then use the unmarshaller to hydrate the types and
	// ensure that fastssz generated methods match the expected results. It will also test custom
	// HTR methods if provided.
	common.RunSSZStaticTests(t,
		"mainnet", // Network configuration
		"zond",    // Fork or phase
		unmarshaller,
		customHTR) // nil customHTR is acceptable.
}
