package forkchoice

import (
	"fmt"
	"path"
	"testing"

	"github.com/golang/snappy"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
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
	if fork >= version.Bellatrix {
		runTest(t, config, fork, "sync")
	}
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
				case version.Phase0:
					beaconState = unmarshalPhase0State(t, preBeaconStateSSZ)
					beaconBlock = unmarshalPhase0Block(t, blockSSZ)
				case version.Altair:
					beaconState = unmarshalAltairState(t, preBeaconStateSSZ)
					beaconBlock = unmarshalAltairBlock(t, blockSSZ)
				case version.Bellatrix:
					beaconState = unmarshalBellatrixState(t, preBeaconStateSSZ)
					beaconBlock = unmarshalBellatrixBlock(t, blockSSZ)
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
					if step.Block != nil {
						blockFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), fmt.Sprint(*step.Block, ".ssz_snappy"))
						require.NoError(t, err)
						blockSSZ, err := snappy.Decode(nil /* dst */, blockFile)
						require.NoError(t, err)
						var beaconBlock interfaces.ReadOnlySignedBeaconBlock
						switch fork {
						case version.Phase0:
							beaconBlock = unmarshalSignedPhase0Block(t, blockSSZ)
						case version.Altair:
							beaconBlock = unmarshalSignedAltairBlock(t, blockSSZ)
						case version.Bellatrix:
							beaconBlock = unmarshalSignedBellatrixBlock(t, blockSSZ)
						case version.Capella:
							beaconBlock = unmarshalSignedCapellaBlock(t, blockSSZ)
						default:
							t.Fatalf("unknown fork version: %v", fork)
						}
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
					if step.PowBlock != nil {
						powBlockFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), fmt.Sprint(*step.PowBlock, ".ssz_snappy"))
						require.NoError(t, err)
						p, err := snappy.Decode(nil /* dst */, powBlockFile)
						require.NoError(t, err)
						pb := &zondpb.PowBlock{}
						require.NoError(t, pb.UnmarshalSSZ(p), "Failed to unmarshal")
						builder.PoWBlock(pb)
					}
					builder.Check(t, step.Check)
				}
			})
		}
	}
}

func unmarshalPhase0State(t *testing.T, raw []byte) state.BeaconState {
	base := &zondpb.BeaconState{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoPhase0(base)
	require.NoError(t, err)
	return st
}

func unmarshalPhase0Block(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &zondpb.BeaconBlock{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlock{Block: base, Signature: make([]byte, dilithium2.CryptoBytes)})
	require.NoError(t, err)
	return blk
}

func unmarshalSignedPhase0Block(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &zondpb.SignedBeaconBlock{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(base)
	require.NoError(t, err)
	return blk
}

func unmarshalAltairState(t *testing.T, raw []byte) state.BeaconState {
	base := &zondpb.BeaconStateAltair{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoAltair(base)
	require.NoError(t, err)
	return st
}

func unmarshalAltairBlock(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &zondpb.BeaconBlockAltair{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockAltair{Block: base, Signature: make([]byte, dilithium2.CryptoBytes)})
	require.NoError(t, err)
	return blk
}

func unmarshalSignedAltairBlock(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &zondpb.SignedBeaconBlockAltair{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(base)
	require.NoError(t, err)
	return blk
}

func unmarshalBellatrixState(t *testing.T, raw []byte) state.BeaconState {
	base := &zondpb.BeaconStateBellatrix{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	st, err := state_native.InitializeFromProtoBellatrix(base)
	require.NoError(t, err)
	return st
}

func unmarshalBellatrixBlock(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &zondpb.BeaconBlockBellatrix{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockBellatrix{Block: base, Signature: make([]byte, dilithium2.CryptoBytes)})
	require.NoError(t, err)
	return blk
}

func unmarshalSignedBellatrixBlock(t *testing.T, raw []byte) interfaces.ReadOnlySignedBeaconBlock {
	base := &zondpb.SignedBeaconBlockBellatrix{}
	require.NoError(t, base.UnmarshalSSZ(raw))
	blk, err := blocks.NewSignedBeaconBlock(base)
	require.NoError(t, err)
	return blk
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
	blk, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockCapella{Block: base, Signature: make([]byte, dilithium2.CryptoBytes)})
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
