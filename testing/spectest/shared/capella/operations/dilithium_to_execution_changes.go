package operations

import (
	"context"
	"path"
	"testing"

	"github.com/golang/snappy"
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/spectest/utils"
	"github.com/theQRL/qrysm/testing/util"
)

func RunDilithiumToExecutionChangeTest(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, "capella", "operations/dilithium_to_execution_change/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, "capella", "operations/dilithium_to_execution_change/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			changeFile, err := util.BazelFileBytes(folderPath, "address_change.ssz_snappy")
			require.NoError(t, err)
			changeSSZ, err := snappy.Decode(nil /* dst */, changeFile)
			require.NoError(t, err, "Failed to decompress")
			change := &zondpb.SignedDilithiumToExecutionChange{}
			require.NoError(t, change.UnmarshalSSZ(changeSSZ), "Failed to unmarshal")

			body := &zondpb.BeaconBlockBodyCapella{
				DilithiumToExecutionChanges: []*zondpb.SignedDilithiumToExecutionChange{change},
			}
			RunBlockOperationTest(t, folderPath, body, func(ctx context.Context, s state.BeaconState, b interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				st, err := blocks.ProcessDilithiumToExecutionChanges(s, b)
				if err != nil {
					return nil, err
				}
				changes, err := b.Block().Body().DilithiumToExecutionChanges()
				if err != nil {
					return nil, err
				}
				cSet, err := blocks.DilithiumChangesSignatureBatch(st, changes)
				if err != nil {
					return nil, err
				}
				ok, err := cSet.Verify()
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, errors.New("signature did not verify")
				}
				return st, nil
			})
		})
	}
}
