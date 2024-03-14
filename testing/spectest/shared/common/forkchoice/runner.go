package forkchoice

import (
	"fmt"
	"path"
	"testing"

	"github.com/golang/snappy"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/spectest/utils"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func init() {
	transition.SkipSlotCache.Disable()
}

// Run executes "forkchoice"  and "sync" test.
func Run(t *testing.T, config string, fork int) {
	runTest(t, config, fork, "fork_choice")
	runTest(t, config, fork, "sync")
}

func runTest(t *testing.T, config string, fork int, basePath string) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, _ := utils.TestFolders(t, config, version.String(fork), basePath)
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, version.String(fork), basePath)
	}

	for _, folder := range testFolders {
		folderPath := path.Join(basePath, folder.Name(), "pyspec_tests")
		testFolders, testsFolderPath := utils.TestFolders(t, config, version.String(fork), folderPath)
		if len(testFolders) == 0 {
			t.Fatalf("No test folders found for %s/%s/%s", config, version.String(fork), folderPath)
		}

		for _, folder := range testFolders {
			t.Run(folder.Name(), func(t *testing.T) {
				preStepsFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "steps.yaml")
				require.NoError(t, err)
				var steps []Step
				require.NoError(t, utils.UnmarshalYaml(preStepsFile, &steps))

				preBeaconStateFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "anchor_state.ssz_snappy")
				require.NoError(t, err)
				preBeaconStateSSZ, err := snappy.Decode(nil /* dst */, preBeaconStateFile)
				require.NoError(t, err)

				blockFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "anchor_block.ssz_snappy")
				require.NoError(t, err)
				blockSSZ, err := snappy.Decode(nil /* dst */, blockFile)
				require.NoError(t, err)

				var beaconState state.BeaconState
				var beaconBlock interfaces.ReadOnlySignedBeaconBlock
				switch fork {
				case version.Capella:
					beaconState = unmarshalCapellaState(t, preBeaconStateSSZ)
					beaconBlock = unmarshalCapellaBlock(t, blockSSZ)
				default:
					t.Fatalf("unknown fork version: %v", fork)
				}

				builder := NewBuilder(t, beaconState, beaconBlock)

				for _, step := range steps {
					if step.Tick != nil {
						builder.Tick(t, int64(*step.Tick))
					}
					var beaconBlock interfaces.ReadOnlySignedBeaconBlock
					if step.Block != nil {
						blockFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), fmt.Sprint(*step.Block, ".ssz_snappy"))
						require.NoError(t, err)
						blockSSZ, err := snappy.Decode(nil /* dst */, blockFile)
						require.NoError(t, err)
						switch fork {
						case version.Capella:
							beaconBlock = unmarshalSignedCapellaBlock(t, blockSSZ)
						default:
							t.Fatalf("unknown fork version: %v", fork)
						}
					}
					if beaconBlock != nil {
						if step.Valid != nil && !*step.Valid {
							builder.InvalidBlock(t, beaconBlock)
						} else {
							builder.ValidBlock(t, beaconBlock)
						}
					}
					if step.AttesterSlashing != nil {
						slashingFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), fmt.Sprint(*step.AttesterSlashing, ".ssz_snappy"))
						require.NoError(t, err)
						slashingSSZ, err := snappy.Decode(nil /* dst */, slashingFile)
						require.NoError(t, err)
						slashing := &zondpb.AttesterSlashing{}
						require.NoError(t, slashing.UnmarshalSSZ(slashingSSZ), "Failed to unmarshal")
						builder.AttesterSlashing(slashing)
					}
					if step.Attestation != nil {
						attFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), fmt.Sprint(*step.Attestation, ".ssz_snappy"))
						require.NoError(t, err)
						attSSZ, err := snappy.Decode(nil /* dst */, attFile)
						require.NoError(t, err)
						att := &zondpb.Attestation{}
						require.NoError(t, att.UnmarshalSSZ(attSSZ), "Failed to unmarshal")
						builder.Attestation(t, att)
					}
					if step.PayloadStatus != nil {
						require.NoError(t, builder.SetPayloadStatus(step.PayloadStatus))
					}
					/*
						if step.PowBlock != nil {
							powBlockFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), fmt.Sprint(*step.PowBlock, ".ssz_snappy"))
							require.NoError(t, err)
							p, err := snappy.Decode(nil, powBlockFile)
							require.NoError(t, err)
							pb := &zondpb.PowBlock{}
							require.NoError(t, pb.UnmarshalSSZ(p), "Failed to unmarshal")
							builder.PoWBlock(pb)
						}
					*/
					builder.Check(t, step.Check)
				}
			})
		}
	}
}

func unmarshalCapellaState(t *testing.T, raw []byte) state.BeaconState {
	base := &zondpb.BeaconStateCapella{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoCapella(base)
	require.NoError(t, err)
	return st
}

func unmarshalCapellaBlock(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &zondpb.BeaconBlockCapella{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockCapella{Block: base, Signature: make([]byte, field_params.DilithiumSignatureLength)})
	require.NoError(t, err)
	return blk
}

func unmarshalSignedCapellaBlock(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &zondpb.SignedBeaconBlockCapella{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(base)
	require.NoError(t, err)
	return blk
}
