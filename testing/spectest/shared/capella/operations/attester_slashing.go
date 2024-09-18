package operations

import (
	"context"
	"path"
	"testing"

	"github.com/golang/snappy"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/validators"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/spectest/utils"
	"github.com/theQRL/qrysm/testing/util"
)

func RunAttesterSlashingTest(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, "capella", "operations/attester_slashing/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, "capella", "operations/attester_slashing/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			attSlashingFile, err := util.BazelFileBytes(folderPath, "attester_slashing.ssz_snappy")
			require.NoError(t, err)
			attSlashingSSZ, err := snappy.Decode(nil /* dst */, attSlashingFile)
			require.NoError(t, err, "Failed to decompress")
			attSlashing := &zondpb.AttesterSlashing{}
			require.NoError(t, attSlashing.UnmarshalSSZ(attSlashingSSZ), "Failed to unmarshal")

			body := &zondpb.BeaconBlockBodyCapella{AttesterSlashings: []*zondpb.AttesterSlashing{attSlashing}}
			RunBlockOperationTest(t, folderPath, body, func(ctx context.Context, s state.BeaconState, b interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				return blocks.ProcessAttesterSlashings(ctx, s, b.Block().Body().AttesterSlashings(), validators.SlashValidator)
			})
		})
	}
}
