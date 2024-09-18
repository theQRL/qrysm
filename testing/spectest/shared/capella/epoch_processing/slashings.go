package epoch_processing

import (
	"path"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/epoch"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/spectest/utils"
)

// RunSlashingsTests executes "epoch_processing/slashings" tests.
func RunSlashingsTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "capella", "epoch_processing/slashings/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, "capella", "epoch_processing/slashings/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			helpers.ClearCache()
			RunEpochOperationTest(t, folderPath, processSlashingsWrapper)
		})
	}
}

func processSlashingsWrapper(t *testing.T, st state.BeaconState) (state.BeaconState, error) {
	st, err := epoch.ProcessSlashings(st, params.BeaconConfig().ProportionalSlashingMultiplier)
	require.NoError(t, err, "Could not process slashings")
	return st, nil
}
