package operations

import (
	"context"
	"path"
	"testing"

	"github.com/golang/snappy"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/spectest/utils"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func RunSyncCommitteeTest(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, "bellatrix", "operations/sync_aggregate/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, "bellatrix", "operations/sync_aggregate/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			syncCommitteeFile, err := util.BazelFileBytes(folderPath, "sync_aggregate.ssz_snappy")
			require.NoError(t, err)
			syncCommitteeSSZ, err := snappy.Decode(nil /* dst */, syncCommitteeFile)
			require.NoError(t, err, "Failed to decompress")
			sc := &zondpb.SyncAggregate{}
			require.NoError(t, sc.UnmarshalSSZ(syncCommitteeSSZ), "Failed to unmarshal")

			body := &zondpb.BeaconBlockBodyBellatrix{SyncAggregate: sc}
			RunBlockOperationTest(t, folderPath, body, func(ctx context.Context, s state.BeaconState, b interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				st, _, err := altair.ProcessSyncAggregate(context.Background(), s, body.SyncAggregate)
				if err != nil {
					return nil, err
				}
				return st, nil
			})
		})
	}
}
