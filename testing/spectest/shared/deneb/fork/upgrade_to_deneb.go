package fork

import (
	"path"
	"testing"

	"github.com/golang/snappy"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/deneb"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/spectest/utils"
	"github.com/theQRL/qrysm/v4/testing/util"
	"google.golang.org/protobuf/proto"
)

// RunUpgradeToDeneb is a helper function that runs Deneb's fork spec tests.
// It unmarshals a pre- and post-state to check `UpgradeToDeneb` comply with spec implementation.
func RunUpgradeToDeneb(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	testFolders, testsFolderPath := utils.TestFolders(t, config, "deneb", "fork/fork/pyspec_tests")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			helpers.ClearCache()
			folderPath := path.Join(testsFolderPath, folder.Name())

			preStateFile, err := util.BazelFileBytes(path.Join(folderPath, "pre.ssz_snappy"))
			require.NoError(t, err)
			preStateSSZ, err := snappy.Decode(nil /* dst */, preStateFile)
			require.NoError(t, err, "Failed to decompress")
			preStateBase := &zondpb.BeaconStateCapella{}
			if err := preStateBase.UnmarshalSSZ(preStateSSZ); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}
			preState, err := state_native.InitializeFromProtoCapella(preStateBase)
			require.NoError(t, err)
			postState, err := deneb.UpgradeToDeneb(preState)
			require.NoError(t, err)
			postStateFromFunction, err := state_native.ProtobufBeaconStateDeneb(postState.ToProtoUnsafe())
			require.NoError(t, err)

			postStateFile, err := util.BazelFileBytes(path.Join(folderPath, "post.ssz_snappy"))
			require.NoError(t, err)
			postStateSSZ, err := snappy.Decode(nil /* dst */, postStateFile)
			require.NoError(t, err, "Failed to decompress")
			postStateFromFile := &zondpb.BeaconStateDeneb{}
			if err := postStateFromFile.UnmarshalSSZ(postStateSSZ); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if !proto.Equal(postStateFromFile, postStateFromFunction) {
				t.Log(postStateFromFile.LatestExecutionPayloadHeader)
				t.Log(postStateFromFunction.LatestExecutionPayloadHeader)
				t.Fatal("Post state does not match expected")
			}
		})
	}
}
