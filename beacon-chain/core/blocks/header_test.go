package blocks_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/validators"
	p2ptypes "github.com/theQRL/qrysm/beacon-chain/p2p/types"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/hash"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/protobuf/proto"
)

func init() {
	logrus.SetOutput(io.Discard) // Ignore "validator activated" logs
}

func TestProcessBlockHeaderNoVerify_InvalidProposerIndices(t *testing.T) {
	ctx := context.Background()
	beaconState, _ := util.DeterministicGenesisStateZond(t, 8)
	require.NoError(t, beaconState.SetSlot(1))
	parentHeader := beaconState.LatestBlockHeader()
	parentRoot, err := parentHeader.HashTreeRoot()
	require.NoError(t, err)
	proposerIndex, err := helpers.BeaconProposerIndex(ctx, beaconState)
	require.NoError(t, err)
	bodyRoot := make([]byte, 32)

	// Establish that the header is otherwise valid without a state-root check.
	_, err = blocks.ProcessBlockHeaderNoVerify(ctx, beaconState.Copy(), beaconState.Slot(), proposerIndex, parentRoot[:], bodyRoot)
	require.NoError(t, err)

	for _, tt := range []struct {
		name  string
		index primitives.ValidatorIndex
	}{
		{name: "wrong proposer within registry", index: (proposerIndex + 1) % 8},
		{name: "index equal to registry length", index: 8},
		{name: "index with high bit set", index: 1 << 63},
		{name: "maximum index", index: ^primitives.ValidatorIndex(0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := blocks.ProcessBlockHeaderNoVerify(ctx, beaconState, beaconState.Slot(), tt.index, parentRoot[:], bodyRoot)
			require.ErrorContains(t, fmt.Sprintf("proposer index: %d is different than calculated: %d", tt.index, proposerIndex), err)
			require.Equal(t, nil, result)
			require.DeepEqual(t, parentHeader, beaconState.LatestBlockHeader())
		})
	}
}

func TestProcessBlockHeaderNoVerify_CommitteeCacheStateIsolation(t *testing.T) {
	ctx := context.Background()
	cfg := params.BeaconConfig()
	for _, primeHasExit := range []bool{false, true} {
		for _, valid := range []bool{true, false} {
			t.Run(fmt.Sprintf("priming branch has exit=%t/valid proposer=%t", primeHasExit, valid), func(t *testing.T) {
				helpers.ClearCache()
				t.Cleanup(helpers.ClearCache)
				withoutExit, _ := util.DeterministicGenesisStateZond(t, 256)
				requestEpoch := cfg.ShardCommitteePeriod + 5
				require.NoError(t, withoutExit.SetSlot(primitives.Slot(requestEpoch)*cfg.SlotsPerEpoch+3))
				withExit := withoutExit.Copy()
				queue, churn := validators.ValidatorsMaxExitEpochAndChurn(withExit)
				_, epoch, err := validators.InitiateValidatorExit(ctx, withExit, 0, queue, churn)
				require.NoError(t, err)
				slot := primitives.Slot(epoch) * cfg.SlotsPerEpoch
				require.NoError(t, withExit.SetSlot(slot))
				require.NoError(t, withoutExit.SetSlot(slot))
				// Distinct branch roots also exercise the separate proposer cache.
				historyIndex := uint64((slot - 1) % cfg.SlotsPerHistoricalRoot)
				require.NoError(t, withExit.UpdateStateRootAtIndex(historyIndex, [32]byte{1}))
				require.NoError(t, withoutExit.UpdateStateRootAtIndex(historyIndex, [32]byte{2}))
				prime, target := withoutExit, withExit
				if primeHasExit {
					prime, target = withExit, withoutExit
				}
				primeSeed, err := helpers.Seed(prime, epoch, cfg.DomainBeaconAttester)
				require.NoError(t, err)
				targetSeed, err := helpers.Seed(target, epoch, cfg.DomainBeaconAttester)
				require.NoError(t, err)
				require.Equal(t, primeSeed, targetSeed)

				// Derive proposer candidates without consulting either cache.
				allIndices := make([]primitives.ValidatorIndex, 256)
				for i := range allIndices {
					allIndices[i] = primitives.ValidatorIndex(i)
				}
				primeIndices, targetIndices := allIndices, allIndices[1:]
				if primeHasExit {
					primeIndices, targetIndices = targetIndices, primeIndices
				}
				seed, err := helpers.Seed(target, epoch, cfg.DomainBeaconProposer)
				require.NoError(t, err)
				var correct, foreign primitives.ValidatorIndex
				for offset := primitives.Slot(0); offset < cfg.SlotsPerEpoch; offset++ {
					slot = primitives.Slot(epoch)*cfg.SlotsPerEpoch + offset
					slotSeed := hash.Hash(append(seed[:], bytesutil.Bytes8(uint64(slot))...))
					correct, err = helpers.ComputeProposerIndex(target, targetIndices, slotSeed)
					require.NoError(t, err)
					foreign, err = helpers.ComputeProposerIndex(target, primeIndices, slotSeed)
					require.NoError(t, err)
					if correct != foreign {
						break
					}
				}
				require.NotEqual(t, correct, foreign, "fixture must assign different proposers")
				require.NoError(t, target.SetSlot(slot))
				parentRoot, err := target.LatestBlockHeader().HashTreeRoot()
				require.NoError(t, err)
				proposer := correct
				if !valid {
					proposer = foreign
				}
				for _, warm := range []bool{false, true} {
					helpers.ClearCache()
					if warm {
						require.NoError(t, helpers.UpdateCommitteeCache(ctx, prime, epoch))
					}
					// Repeat to check that proposer-cache population remains correct.
					for attempt := 0; attempt < 2; attempt++ {
						_, err = blocks.ProcessBlockHeaderNoVerify(ctx, target.Copy(), slot, proposer, parentRoot[:], make([]byte, 32))
						if valid {
							require.NoError(t, err, "warm=%t attempt=%d", warm, attempt)
						} else {
							require.ErrorContains(t, fmt.Sprintf("proposer index: %d is different than calculated: %d", proposer, correct), err)
						}
					}
				}
			})
		}
	}
}

func TestProcessBlockHeader_ImproperBlockSlot(t *testing.T) {
	helpers.ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			PublicKey:           make([]byte, 32),
			WithdrawalRecipient: make([]byte, 64),
			ExitEpoch:           params.BeaconConfig().FarFutureEpoch,
			Slashed:             true,
		}
	}

	state, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, state.SetSlot(10))
	require.NoError(t, state.SetValidators(validators))
	require.NoError(t, state.SetLatestBlockHeader(util.HydrateBeaconHeader(&qrysmpb.BeaconBlockHeader{
		Slot: 10, // Must be less than block.Slot
	})))

	latestBlockSignedRoot, err := state.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)

	currentEpoch := time.CurrentEpoch(state)
	priv, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	pID, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	block := util.NewBeaconBlockZond()
	block.Block.ProposerIndex = pID
	block.Block.Slot = 10
	block.Block.Body.RandaoReveal = bytesutil.PadTo([]byte{'A', 'B', 'C'}, field_params.RandaoRevealLength)
	block.Block.ParentRoot = latestBlockSignedRoot[:]
	block.Signature, err = signing.ComputeDomainAndSign(state, currentEpoch, block.Block, params.BeaconConfig().DomainBeaconProposer, priv)
	require.NoError(t, err)

	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	validators[proposerIdx].Slashed = false
	validators[proposerIdx].PublicKey = priv.PublicKey().Marshal()
	err = state.UpdateValidatorAtIndex(proposerIdx, validators[proposerIdx])
	require.NoError(t, err)

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = blocks.ProcessBlockHeader(context.Background(), state, wsb)
	assert.ErrorContains(t, "block.Slot 10 must be greater than state.LatestBlockHeader.Slot 10", err)
}

func TestProcessBlockHeader_WrongProposerSig(t *testing.T) {

	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	require.NoError(t, beaconState.SetLatestBlockHeader(util.HydrateBeaconHeader(&qrysmpb.BeaconBlockHeader{
		Slot: 9,
	})))
	require.NoError(t, beaconState.SetSlot(10))

	lbhdr, err := beaconState.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)

	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), beaconState)
	require.NoError(t, err)

	block := util.NewBeaconBlockZond()
	block.Block.ProposerIndex = proposerIdx
	block.Block.Slot = 10
	block.Block.Body.RandaoReveal = bytesutil.PadTo([]byte{'A', 'B', 'C'}, field_params.RandaoRevealLength)
	block.Block.ParentRoot = lbhdr[:]
	block.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, block.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx+1])
	require.NoError(t, err)

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = blocks.ProcessBlockHeader(context.Background(), beaconState, wsb)
	want := "signature did not verify"
	assert.ErrorContains(t, want, err)
}

func TestProcessBlockHeader_DifferentSlots(t *testing.T) {
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			PublicKey:           make([]byte, 32),
			WithdrawalRecipient: make([]byte, 64),
			ExitEpoch:           params.BeaconConfig().FarFutureEpoch,
			Slashed:             true,
		}
	}

	state, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))
	require.NoError(t, state.SetSlot(10))
	require.NoError(t, state.SetLatestBlockHeader(util.HydrateBeaconHeader(&qrysmpb.BeaconBlockHeader{
		Slot: 9,
	})))

	lbhsr, err := state.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)
	currentEpoch := time.CurrentEpoch(state)

	priv, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	sszBytes := p2ptypes.SSZBytes("hello")
	blockSig, err := signing.ComputeDomainAndSign(state, currentEpoch, &sszBytes, params.BeaconConfig().DomainBeaconProposer, priv)
	require.NoError(t, err)
	block := util.HydrateSignedBeaconBlockZond(&qrysmpb.SignedBeaconBlockZond{
		Block: &qrysmpb.BeaconBlockZond{
			Slot:       1,
			ParentRoot: lbhsr[:],
		},
		Signature: blockSig,
	})

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = blocks.ProcessBlockHeader(context.Background(), state, wsb)
	want := "is different than block slot"
	assert.ErrorContains(t, want, err)
}

func TestProcessBlockHeader_PreviousBlockRootNotSignedRoot(t *testing.T) {
	helpers.ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			PublicKey:           make([]byte, 48),
			WithdrawalRecipient: make([]byte, 64),
			ExitEpoch:           params.BeaconConfig().FarFutureEpoch,
			Slashed:             true,
		}
	}

	state, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))
	require.NoError(t, state.SetSlot(10))
	bh := state.LatestBlockHeader()
	bh.Slot = 9
	require.NoError(t, state.SetLatestBlockHeader(bh))
	currentEpoch := time.CurrentEpoch(state)
	priv, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	sszBytes := p2ptypes.SSZBytes("hello")
	blockSig, err := signing.ComputeDomainAndSign(state, currentEpoch, &sszBytes, params.BeaconConfig().DomainBeaconProposer, priv)
	require.NoError(t, err)
	pID, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	block := util.NewBeaconBlockZond()
	block.Block.Slot = 10
	block.Block.ProposerIndex = pID
	block.Block.Body.RandaoReveal = bytesutil.PadTo([]byte{'A', 'B', 'C'}, 96)
	block.Block.ParentRoot = bytesutil.PadTo([]byte{'A'}, 32)
	block.Signature = blockSig

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = blocks.ProcessBlockHeader(context.Background(), state, wsb)
	want := "does not match"
	assert.ErrorContains(t, want, err)
}

func TestProcessBlockHeader_SlashedProposer(t *testing.T) {
	helpers.ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			PublicKey:           make([]byte, field_params.MLDSA87PubkeyLength),
			WithdrawalRecipient: make([]byte, 64),
			ExitEpoch:           params.BeaconConfig().FarFutureEpoch,
			Slashed:             true,
		}
	}

	state, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))
	require.NoError(t, state.SetSlot(10))
	bh := state.LatestBlockHeader()
	bh.Slot = 9
	require.NoError(t, state.SetLatestBlockHeader(bh))
	parentRoot, err := state.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)
	currentEpoch := time.CurrentEpoch(state)
	priv, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	sszBytes := p2ptypes.SSZBytes("hello")
	blockSig, err := signing.ComputeDomainAndSign(state, currentEpoch, &sszBytes, params.BeaconConfig().DomainBeaconProposer, priv)
	require.NoError(t, err)

	pID, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	block := util.NewBeaconBlockZond()
	block.Block.Slot = 10
	block.Block.ProposerIndex = pID
	block.Block.Body.RandaoReveal = bytesutil.PadTo([]byte{'A', 'B', 'C'}, 96)
	block.Block.ParentRoot = parentRoot[:]
	block.Signature = blockSig

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = blocks.ProcessBlockHeader(context.Background(), state, wsb)
	want := "was previously slashed"
	assert.ErrorContains(t, want, err)
}

func TestProcessBlockHeader_OK(t *testing.T) {
	helpers.ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			PublicKey:           make([]byte, 32),
			WithdrawalRecipient: make([]byte, 64),
			ExitEpoch:           params.BeaconConfig().FarFutureEpoch,
			Slashed:             true,
		}
	}

	state, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))
	require.NoError(t, state.SetSlot(10))
	require.NoError(t, state.SetLatestBlockHeader(util.HydrateBeaconHeader(&qrysmpb.BeaconBlockHeader{
		Slot: 9,
	})))

	latestBlockSignedRoot, err := state.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)

	currentEpoch := time.CurrentEpoch(state)
	priv, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	pID, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	block := util.NewBeaconBlockZond()
	block.Block.ProposerIndex = pID
	block.Block.Slot = 10
	block.Block.Body.RandaoReveal = bytesutil.PadTo([]byte{'A', 'B', 'C'}, field_params.RandaoRevealLength)
	block.Block.ParentRoot = latestBlockSignedRoot[:]
	block.Signature, err = signing.ComputeDomainAndSign(state, currentEpoch, block.Block, params.BeaconConfig().DomainBeaconProposer, priv)
	require.NoError(t, err)
	bodyRoot, err := block.Block.Body.HashTreeRoot()
	require.NoError(t, err, "Failed to hash block bytes got")

	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	validators[proposerIdx].Slashed = false
	validators[proposerIdx].PublicKey = priv.PublicKey().Marshal()
	err = state.UpdateValidatorAtIndex(proposerIdx, validators[proposerIdx])
	require.NoError(t, err)

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	newState, err := blocks.ProcessBlockHeader(context.Background(), state, wsb)
	require.NoError(t, err, "Failed to process block header got")
	var zeroHash [32]byte
	nsh := newState.LatestBlockHeader()
	expected := &qrysmpb.BeaconBlockHeader{
		ProposerIndex: pID,
		Slot:          block.Block.Slot,
		ParentRoot:    latestBlockSignedRoot[:],
		BodyRoot:      bodyRoot[:],
		StateRoot:     zeroHash[:],
	}
	assert.Equal(t, true, proto.Equal(nsh, expected), "Expected %v, received %v", expected, nsh)
}

func TestBlockSignatureSet_OK(t *testing.T) {
	helpers.ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			PublicKey:           make([]byte, 32),
			WithdrawalRecipient: make([]byte, 64),
			ExitEpoch:           params.BeaconConfig().FarFutureEpoch,
			Slashed:             true,
		}
	}

	state, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))
	require.NoError(t, state.SetSlot(10))
	require.NoError(t, state.SetLatestBlockHeader(util.HydrateBeaconHeader(&qrysmpb.BeaconBlockHeader{
		Slot:          9,
		ProposerIndex: 0,
	})))

	latestBlockSignedRoot, err := state.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)

	currentEpoch := time.CurrentEpoch(state)
	priv, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	pID, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	block := util.NewBeaconBlockZond()
	block.Block.Slot = 10
	block.Block.ProposerIndex = pID
	block.Block.Body.RandaoReveal = bytesutil.PadTo([]byte{'A', 'B', 'C'}, field_params.RandaoRevealLength)
	block.Block.ParentRoot = latestBlockSignedRoot[:]
	block.Signature, err = signing.ComputeDomainAndSign(state, currentEpoch, block.Block, params.BeaconConfig().DomainBeaconProposer, priv)
	require.NoError(t, err)
	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	validators[proposerIdx].Slashed = false
	validators[proposerIdx].PublicKey = priv.PublicKey().Marshal()
	err = state.UpdateValidatorAtIndex(proposerIdx, validators[proposerIdx])
	require.NoError(t, err)
	set, err := blocks.BlockSignatureBatch(state, block.Block.ProposerIndex, block.Signature, block.Block.HashTreeRoot)
	require.NoError(t, err)

	verified, err := set.Verify()
	require.NoError(t, err)
	assert.Equal(t, true, verified, "Block signature set returned a set which was unable to be verified")
}
