package epoch_processing

import (
	"path"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/epoch"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/spectest/utils"
)

// RunExecutionDataResetTests executes "epoch_processing/execution_data_reset" tests.
func RunExecutionDataResetTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "zond", "epoch_processing/execution_data_reset/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, "zond", "epoch_processing/execution_data_reset/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			RunEpochOperationTest(t, folderPath, processExecutionDataResetWrapper)
		})
	}
}

func processExecutionDataResetWrapper(t *testing.T, st state.BeaconState) (state.BeaconState, error) {
	st, err := epoch.ProcessExecutionDataReset(st)
	require.NoError(t, err, "Could not process final updates")
	return st, nil
}
