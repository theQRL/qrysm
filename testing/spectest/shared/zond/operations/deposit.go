package operations

import (
	"context"
	"path"
	"testing"

	"github.com/golang/snappy"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/spectest/utils"
	"github.com/theQRL/qrysm/testing/util"
)

func RunDepositTest(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, "zond", "operations/deposit/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, "zond", "operations/deposit/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			depositFile, err := util.BazelFileBytes(folderPath, "deposit.ssz_snappy")
			require.NoError(t, err)
			depositSSZ, err := snappy.Decode(nil /* dst */, depositFile)
			require.NoError(t, err, "Failed to decompress")
			deposit := &qrysmpb.Deposit{}
			require.NoError(t, deposit.UnmarshalSSZ(depositSSZ), "Failed to unmarshal")

			body := &qrysmpb.BeaconBlockBodyZond{Deposits: []*qrysmpb.Deposit{deposit}}
			processDepositsFunc := func(ctx context.Context, s state.BeaconState, b interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				return altair.ProcessDeposits(ctx, s, b.Block().Body().Deposits())
			}
			RunBlockOperationTest(t, folderPath, body, processDepositsFunc)
		})
	}
}
