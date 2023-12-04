package transition_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/time"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	consensusblocks "github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/crypto/bls"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func init() {
	transition.SkipSlotCache.Disable()
}

func TestExecuteStateTransition_IncorrectSlot(t *testing.T) {
	base := &zondpb.BeaconState{
		Slot: 5,
	}
	beaconState, err := state_native.InitializeFromProtoPhase0(base)
	require.NoError(t, err)
	block := &zondpb.SignedBeaconBlock{
		Block: &zondpb.BeaconBlock{
			Slot: 4,
			Body: &zondpb.BeaconBlockBody{},
		},
	}
	want := "expected state.slot"
	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = transition.ExecuteStateTransition(context.Background(), beaconState, wsb)
	assert.ErrorContains(t, want, err)
}

func TestExecuteStateTransition_FullProcess(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)

	eth1Data := &zondpb.Eth1Data{
		DepositCount: 100,
		DepositRoot:  bytesutil.PadTo([]byte{2}, 32),
		BlockHash:    make([]byte, 32),
	}
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch-1))
	e := beaconState.Eth1Data()
	e.DepositCount = 100
	require.NoError(t, beaconState.SetEth1Data(e))
	bh := beaconState.LatestBlockHeader()
	bh.Slot = beaconState.Slot()
	require.NoError(t, beaconState.SetLatestBlockHeader(bh))
	require.NoError(t, beaconState.SetEth1DataVotes([]*zondpb.Eth1Data{eth1Data}))

	oldMix, err := beaconState.RandaoMixAtIndex(1)
	require.NoError(t, err)

	require.NoError(t, beaconState.SetSlot(beaconState.Slot()+1))
	epoch := time.CurrentEpoch(beaconState)
	randaoReveal, err := util.RandaoReveal(beaconState, epoch, privKeys)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(beaconState.Slot()-1))

	nextSlotState, err := transition.ProcessSlots(context.Background(), beaconState.Copy(), beaconState.Slot()+1)
	require.NoError(t, err)
	parentRoot, err := nextSlotState.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)
	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), nextSlotState)
	require.NoError(t, err)
	block := util.NewBeaconBlock()
	block.Block.ProposerIndex = proposerIdx
	block.Block.Slot = beaconState.Slot() + 1
	block.Block.ParentRoot = parentRoot[:]
	block.Block.Body.RandaoReveal = randaoReveal
	block.Block.Body.Eth1Data = eth1Data

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	stateRoot, err := transition.CalculateStateRoot(context.Background(), beaconState, wsb)
	require.NoError(t, err)

	block.Block.StateRoot = stateRoot[:]

	sig, err := util.BlockSignature(beaconState, block.Block, privKeys)
	require.NoError(t, err)
	block.Signature = sig.Marshal()

	wsb, err = consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	beaconState, err = transition.ExecuteStateTransition(context.Background(), beaconState, wsb)
	require.NoError(t, err)

	assert.Equal(t, params.BeaconConfig().SlotsPerEpoch, beaconState.Slot(), "Unexpected Slot number")

	mix, err := beaconState.RandaoMixAtIndex(1)
	require.NoError(t, err)
	assert.DeepNotEqual(t, oldMix, mix, "Did not expect new and old randao mix to equal")
}

func TestProcessBlock_IncorrectProcessExits(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisState(t, 100)

	proposerSlashings := []*zondpb.ProposerSlashing{
		{
			Header_1: util.HydrateSignedBeaconHeader(&zondpb.SignedBeaconBlockHeader{
				Header: &zondpb.BeaconBlockHeader{
					ProposerIndex: 3,
					Slot:          1,
				},
				Signature: bytesutil.PadTo([]byte("A"), 96),
			}),
			Header_2: util.HydrateSignedBeaconHeader(&zondpb.SignedBeaconBlockHeader{
				Header: &zondpb.BeaconBlockHeader{
					ProposerIndex: 3,
					Slot:          1,
				},
				Signature: bytesutil.PadTo([]byte("B"), 96),
			}),
		},
	}
	attesterSlashings := []*zondpb.AttesterSlashing{
		{
			Attestation_1: &zondpb.IndexedAttestation{
				Data:             util.HydrateAttestationData(&zondpb.AttestationData{}),
				AttestingIndices: []uint64{0, 1},
				Signature:        make([]byte, 96),
			},
			Attestation_2: &zondpb.IndexedAttestation{
				Data:             util.HydrateAttestationData(&zondpb.AttestationData{}),
				AttestingIndices: []uint64{0, 1},
				Signature:        make([]byte, 96),
			},
		},
	}
	var blockRoots [][]byte
	for i := uint64(0); i < uint64(params.BeaconConfig().SlotsPerHistoricalRoot); i++ {
		blockRoots = append(blockRoots, []byte{byte(i)})
	}
	require.NoError(t, beaconState.SetBlockRoots(blockRoots))
	blockAtt := util.HydrateAttestation(&zondpb.Attestation{
		Data: &zondpb.AttestationData{
			Target: &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte("hello-world"), 32)},
		},
		AggregationBits: bitfield.Bitlist{0xC0, 0xC0, 0xC0, 0xC0, 0x01},
	})
	attestations := []*zondpb.Attestation{blockAtt}
	var exits []*zondpb.SignedVoluntaryExit
	for i := uint64(0); i < params.BeaconConfig().MaxVoluntaryExits+1; i++ {
		exits = append(exits, &zondpb.SignedVoluntaryExit{})
	}
	genesisBlock := blocks.NewGenesisBlock([]byte{})
	bodyRoot, err := genesisBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	err = beaconState.SetLatestBlockHeader(util.HydrateBeaconHeader(&zondpb.BeaconBlockHeader{
		Slot:       genesisBlock.Block.Slot,
		ParentRoot: genesisBlock.Block.ParentRoot,
		BodyRoot:   bodyRoot[:],
	}))
	require.NoError(t, err)
	parentRoot, err := beaconState.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)
	block := util.NewBeaconBlock()
	block.Block.Slot = 1
	block.Block.ParentRoot = parentRoot[:]
	block.Block.Body.ProposerSlashings = proposerSlashings
	block.Block.Body.Attestations = attestations
	block.Block.Body.AttesterSlashings = attesterSlashings
	block.Block.Body.VoluntaryExits = exits
	block.Block.Body.Eth1Data.DepositRoot = bytesutil.PadTo([]byte{2}, 32)
	block.Block.Body.Eth1Data.BlockHash = bytesutil.PadTo([]byte{3}, 32)
	err = beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)
	cp := beaconState.CurrentJustifiedCheckpoint()
	cp.Root = []byte("hello-world")
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(cp))
	require.NoError(t, beaconState.AppendCurrentEpochAttestations(&zondpb.PendingAttestation{}))
	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = transition.VerifyOperationLengths(context.Background(), beaconState, wsb)
	wanted := "number of voluntary exits (17) in block body exceeds allowed threshold of 16"
	assert.ErrorContains(t, wanted, err)
}

func createFullBlockWithOperations(t *testing.T) (state.BeaconState,
	*zondpb.SignedBeaconBlock, []*zondpb.Attestation, []*zondpb.ProposerSlashing, []*zondpb.SignedVoluntaryExit) {
	beaconState, privKeys := util.DeterministicGenesisState(t, 32)
	genesisBlock := blocks.NewGenesisBlock([]byte{})
	bodyRoot, err := genesisBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	err = beaconState.SetLatestBlockHeader(&zondpb.BeaconBlockHeader{
		Slot:       genesisBlock.Block.Slot,
		ParentRoot: genesisBlock.Block.ParentRoot,
		StateRoot:  params.BeaconConfig().ZeroHash[:],
		BodyRoot:   bodyRoot[:],
	})
	require.NoError(t, err)
	err = beaconState.SetSlashings(make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector))
	require.NoError(t, err)
	cp := beaconState.CurrentJustifiedCheckpoint()
	var mockRoot [32]byte
	copy(mockRoot[:], "hello-world")
	cp.Root = mockRoot[:]
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(cp))
	require.NoError(t, beaconState.AppendCurrentEpochAttestations(&zondpb.PendingAttestation{}))

	proposerSlashIdx := primitives.ValidatorIndex(3)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	err = beaconState.SetSlot(slotsPerEpoch.Mul(uint64(params.BeaconConfig().ShardCommitteePeriod)) + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)

	currentEpoch := time.CurrentEpoch(beaconState)
	header1 := util.HydrateSignedBeaconHeader(&zondpb.SignedBeaconBlockHeader{
		Header: &zondpb.BeaconBlockHeader{
			ProposerIndex: proposerSlashIdx,
			Slot:          1,
			StateRoot:     bytesutil.PadTo([]byte("A"), 32),
		},
	})
	header1.Signature, err = signing.ComputeDomainAndSign(beaconState, currentEpoch, header1.Header, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerSlashIdx])
	require.NoError(t, err)

	header2 := util.HydrateSignedBeaconHeader(&zondpb.SignedBeaconBlockHeader{
		Header: &zondpb.BeaconBlockHeader{
			ProposerIndex: proposerSlashIdx,
			Slot:          1,
			StateRoot:     bytesutil.PadTo([]byte("B"), 32),
		},
	})
	header2.Signature, err = signing.ComputeDomainAndSign(beaconState, time.CurrentEpoch(beaconState), header2.Header, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerSlashIdx])
	require.NoError(t, err)

	proposerSlashings := []*zondpb.ProposerSlashing{
		{
			Header_1: header1,
			Header_2: header2,
		},
	}
	validators := beaconState.Validators()
	validators[proposerSlashIdx].PublicKey = privKeys[proposerSlashIdx].PublicKey().Marshal()
	require.NoError(t, beaconState.SetValidators(validators))

	mockRoot2 := [32]byte{'A'}
	att1 := util.HydrateIndexedAttestation(&zondpb.IndexedAttestation{
		Data: &zondpb.AttestationData{
			Source: &zondpb.Checkpoint{Epoch: 0, Root: mockRoot2[:]},
		},
		AttestingIndices: []uint64{0, 1},
	})
	domain, err := signing.Domain(beaconState.Fork(), currentEpoch, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att1.Data, domain)
	require.NoError(t, err)
	sig0 := privKeys[0].Sign(hashTreeRoot[:])
	sig1 := privKeys[1].Sign(hashTreeRoot[:])
	aggregateSig := bls.AggregateSignatures([]bls.Signature{sig0, sig1})
	att1.Signature = aggregateSig.Marshal()

	mockRoot3 := [32]byte{'B'}
	att2 := util.HydrateIndexedAttestation(&zondpb.IndexedAttestation{
		Data: &zondpb.AttestationData{
			Source: &zondpb.Checkpoint{Epoch: 0, Root: mockRoot3[:]},
			Target: &zondpb.Checkpoint{Epoch: 0, Root: make([]byte, fieldparams.RootLength)},
		},
		AttestingIndices: []uint64{0, 1},
	})

	hashTreeRoot, err = signing.ComputeSigningRoot(att2.Data, domain)
	require.NoError(t, err)
	sig0 = privKeys[0].Sign(hashTreeRoot[:])
	sig1 = privKeys[1].Sign(hashTreeRoot[:])
	aggregateSig = bls.AggregateSignatures([]bls.Signature{sig0, sig1})
	att2.Signature = aggregateSig.Marshal()

	attesterSlashings := []*zondpb.AttesterSlashing{
		{
			Attestation_1: att1,
			Attestation_2: att2,
		},
	}

	var blockRoots [][]byte
	for i := uint64(0); i < uint64(params.BeaconConfig().SlotsPerHistoricalRoot); i++ {
		blockRoots = append(blockRoots, []byte{byte(i)})
	}
	require.NoError(t, beaconState.SetBlockRoots(blockRoots))

	aggBits := bitfield.NewBitlist(1)
	aggBits.SetBitAt(0, true)
	blockAtt := util.HydrateAttestation(&zondpb.Attestation{
		Data: &zondpb.AttestationData{
			Slot:   beaconState.Slot(),
			Target: &zondpb.Checkpoint{Epoch: time.CurrentEpoch(beaconState)},
			Source: &zondpb.Checkpoint{Root: mockRoot[:]}},
		AggregationBits: aggBits,
	})

	committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, blockAtt.Data.Slot, blockAtt.Data.CommitteeIndex)
	assert.NoError(t, err)
	attestingIndices, err := attestation.AttestingIndices(blockAtt.AggregationBits, committee)
	require.NoError(t, err)
	assert.NoError(t, err)
	hashTreeRoot, err = signing.ComputeSigningRoot(blockAtt.Data, domain)
	assert.NoError(t, err)
	sigs := make([]bls.Signature, len(attestingIndices))
	for i, indice := range attestingIndices {
		sig := privKeys[indice].Sign(hashTreeRoot[:])
		sigs[i] = sig
	}
	blockAtt.Signature = bls.AggregateSignatures(sigs).Marshal()

	exit := &zondpb.SignedVoluntaryExit{
		Exit: &zondpb.VoluntaryExit{
			ValidatorIndex: 10,
			Epoch:          0,
		},
	}
	exit.Signature, err = signing.ComputeDomainAndSign(beaconState, currentEpoch, exit.Exit, params.BeaconConfig().DomainVoluntaryExit, privKeys[exit.Exit.ValidatorIndex])
	require.NoError(t, err)

	header := beaconState.LatestBlockHeader()
	prevStateRoot, err := beaconState.HashTreeRoot(context.Background())
	require.NoError(t, err)
	header.StateRoot = prevStateRoot[:]
	require.NoError(t, beaconState.SetLatestBlockHeader(header))
	parentRoot, err := beaconState.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(beaconState.Slot()+1))
	randaoReveal, err := util.RandaoReveal(copied, currentEpoch, privKeys)
	require.NoError(t, err)
	proposerIndex, err := helpers.BeaconProposerIndex(context.Background(), copied)
	require.NoError(t, err)
	block := util.HydrateSignedBeaconBlock(&zondpb.SignedBeaconBlock{
		Block: &zondpb.BeaconBlock{
			ParentRoot:    parentRoot[:],
			Slot:          beaconState.Slot() + 1,
			ProposerIndex: proposerIndex,
			Body: &zondpb.BeaconBlockBody{
				RandaoReveal:      randaoReveal,
				ProposerSlashings: proposerSlashings,
				AttesterSlashings: attesterSlashings,
				Attestations:      []*zondpb.Attestation{blockAtt},
				VoluntaryExits:    []*zondpb.SignedVoluntaryExit{exit},
			},
		},
	})

	sig, err := util.BlockSignature(beaconState, block.Block, privKeys)
	require.NoError(t, err)
	block.Signature = sig.Marshal()

	require.NoError(t, beaconState.SetSlot(block.Block.Slot))
	return beaconState, block, []*zondpb.Attestation{blockAtt}, proposerSlashings, []*zondpb.SignedVoluntaryExit{exit}
}

func TestProcessEpochPrecompute_CanProcess(t *testing.T) {
	epoch := primitives.Epoch(1)

	atts := []*zondpb.PendingAttestation{{Data: &zondpb.AttestationData{Target: &zondpb.Checkpoint{Root: make([]byte, 32)}}, InclusionDelay: 1}}
	slashing := make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector)
	base := &zondpb.BeaconState{
		Slot:                       params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epoch)) + 1,
		BlockRoots:                 make([][]byte, 128),
		Slashings:                  slashing,
		RandaoMixes:                make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
		CurrentEpochAttestations:   atts,
		FinalizedCheckpoint:        &zondpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		JustificationBits:          bitfield.Bitvector4{0x00},
		CurrentJustifiedCheckpoint: &zondpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
	}
	s, err := state_native.InitializeFromProtoPhase0(base)
	require.NoError(t, err)
	require.NoError(t, s.SetValidators([]*zondpb.Validator{}))
	newState, err := transition.ProcessEpochPrecompute(context.Background(), s)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), newState.Slashings()[2], "Unexpected slashed balance")
}

func TestProcessBlock_OverMaxProposerSlashings(t *testing.T) {
	maxSlashings := params.BeaconConfig().MaxProposerSlashings
	b := &zondpb.SignedBeaconBlock{
		Block: &zondpb.BeaconBlock{
			Body: &zondpb.BeaconBlockBody{
				ProposerSlashings: make([]*zondpb.ProposerSlashing, maxSlashings+1),
			},
		},
	}
	want := fmt.Sprintf("number of proposer slashings (%d) in block body exceeds allowed threshold of %d",
		len(b.Block.Body.ProposerSlashings), params.BeaconConfig().MaxProposerSlashings)
	s, err := state_native.InitializeFromProtoUnsafePhase0(&zondpb.BeaconState{})
	require.NoError(t, err)
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = transition.VerifyOperationLengths(context.Background(), s, wsb)
	assert.ErrorContains(t, want, err)
}

func TestProcessBlock_OverMaxAttesterSlashings(t *testing.T) {
	maxSlashings := params.BeaconConfig().MaxAttesterSlashings
	b := &zondpb.SignedBeaconBlock{
		Block: &zondpb.BeaconBlock{
			Body: &zondpb.BeaconBlockBody{
				AttesterSlashings: make([]*zondpb.AttesterSlashing, maxSlashings+1),
			},
		},
	}
	want := fmt.Sprintf("number of attester slashings (%d) in block body exceeds allowed threshold of %d",
		len(b.Block.Body.AttesterSlashings), params.BeaconConfig().MaxAttesterSlashings)
	s, err := state_native.InitializeFromProtoUnsafePhase0(&zondpb.BeaconState{})
	require.NoError(t, err)
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = transition.VerifyOperationLengths(context.Background(), s, wsb)
	assert.ErrorContains(t, want, err)
}

func TestProcessBlock_OverMaxAttestations(t *testing.T) {
	b := &zondpb.SignedBeaconBlock{
		Block: &zondpb.BeaconBlock{
			Body: &zondpb.BeaconBlockBody{
				Attestations: make([]*zondpb.Attestation, params.BeaconConfig().MaxAttestations+1),
			},
		},
	}
	want := fmt.Sprintf("number of attestations (%d) in block body exceeds allowed threshold of %d",
		len(b.Block.Body.Attestations), params.BeaconConfig().MaxAttestations)
	s, err := state_native.InitializeFromProtoUnsafePhase0(&zondpb.BeaconState{})
	require.NoError(t, err)
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = transition.VerifyOperationLengths(context.Background(), s, wsb)
	assert.ErrorContains(t, want, err)
}

func TestProcessBlock_OverMaxVoluntaryExits(t *testing.T) {
	maxExits := params.BeaconConfig().MaxVoluntaryExits
	b := &zondpb.SignedBeaconBlock{
		Block: &zondpb.BeaconBlock{
			Body: &zondpb.BeaconBlockBody{
				VoluntaryExits: make([]*zondpb.SignedVoluntaryExit, maxExits+1),
			},
		},
	}
	want := fmt.Sprintf("number of voluntary exits (%d) in block body exceeds allowed threshold of %d",
		len(b.Block.Body.VoluntaryExits), maxExits)
	s, err := state_native.InitializeFromProtoUnsafePhase0(&zondpb.BeaconState{})
	require.NoError(t, err)
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = transition.VerifyOperationLengths(context.Background(), s, wsb)
	assert.ErrorContains(t, want, err)
}

func TestProcessBlock_IncorrectDeposits(t *testing.T) {
	base := &zondpb.BeaconState{
		Eth1Data:         &zondpb.Eth1Data{DepositCount: 100},
		Eth1DepositIndex: 98,
	}
	s, err := state_native.InitializeFromProtoPhase0(base)
	require.NoError(t, err)
	b := &zondpb.SignedBeaconBlock{
		Block: &zondpb.BeaconBlock{
			Body: &zondpb.BeaconBlockBody{
				Deposits: []*zondpb.Deposit{{}},
			},
		},
	}
	want := fmt.Sprintf("incorrect outstanding deposits in block body, wanted: %d, got: %d",
		s.Eth1Data().DepositCount-s.Eth1DepositIndex(), len(b.Block.Body.Deposits))
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = transition.VerifyOperationLengths(context.Background(), s, wsb)
	assert.ErrorContains(t, want, err)
}

func TestProcessSlots_SameSlotAsParentState(t *testing.T) {
	slot := primitives.Slot(2)
	parentState, err := state_native.InitializeFromProtoPhase0(&zondpb.BeaconState{Slot: slot})
	require.NoError(t, err)

	_, err = transition.ProcessSlots(context.Background(), parentState, slot)
	assert.ErrorContains(t, "expected state.slot 2 < slot 2", err)
}

func TestProcessSlots_LowerSlotAsParentState(t *testing.T) {
	slot := primitives.Slot(2)
	parentState, err := state_native.InitializeFromProtoPhase0(&zondpb.BeaconState{Slot: slot})
	require.NoError(t, err)

	_, err = transition.ProcessSlots(context.Background(), parentState, slot-1)
	assert.ErrorContains(t, "expected state.slot 2 < slot 1", err)
}

func TestProcessSlots_ThroughAltairEpoch(t *testing.T) {
	transition.SkipSlotCache.Disable()
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig()
	conf.AltairForkEpoch = 5
	params.OverrideBeaconConfig(conf)

	st, _ := util.DeterministicGenesisState(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	st, err := transition.ProcessSlots(context.Background(), st, params.BeaconConfig().SlotsPerEpoch*10)
	require.NoError(t, err)
	require.Equal(t, version.Altair, st.Version())

	require.Equal(t, params.BeaconConfig().SlotsPerEpoch*10, st.Slot())

	s, err := st.InactivityScores()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(s)))

	p, err := st.PreviousEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	p, err = st.CurrentEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	sc, err := st.CurrentSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))

	sc, err = st.NextSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))
}

func TestProcessSlots_OnlyAltairEpoch(t *testing.T) {
	transition.SkipSlotCache.Disable()
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig()
	conf.AltairForkEpoch = 5
	params.OverrideBeaconConfig(conf)

	st, _ := util.DeterministicGenesisStateAltair(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	require.NoError(t, st.SetSlot(params.BeaconConfig().SlotsPerEpoch*6))
	st, err := transition.ProcessSlots(context.Background(), st, params.BeaconConfig().SlotsPerEpoch*10)
	require.NoError(t, err)
	require.Equal(t, version.Altair, st.Version())

	require.Equal(t, params.BeaconConfig().SlotsPerEpoch*10, st.Slot())

	s, err := st.InactivityScores()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(s)))

	p, err := st.PreviousEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	p, err = st.CurrentEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	sc, err := st.CurrentSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))

	sc, err = st.NextSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))
}

func TestProcessSlots_OnlyBellatrixEpoch(t *testing.T) {
	transition.SkipSlotCache.Disable()
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig().Copy()
	conf.BellatrixForkEpoch = 5
	params.OverrideBeaconConfig(conf)

	st, _ := util.DeterministicGenesisStateBellatrix(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	require.NoError(t, st.SetSlot(params.BeaconConfig().SlotsPerEpoch*6))
	require.Equal(t, version.Bellatrix, st.Version())
	st, err := transition.ProcessSlots(context.Background(), st, params.BeaconConfig().SlotsPerEpoch*10)
	require.NoError(t, err)
	require.Equal(t, version.Bellatrix, st.Version())

	require.Equal(t, params.BeaconConfig().SlotsPerEpoch*10, st.Slot())

	s, err := st.InactivityScores()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(s)))

	p, err := st.PreviousEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	p, err = st.CurrentEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	sc, err := st.CurrentSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))

	sc, err = st.NextSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))
}

func TestProcessSlots_ThroughBellatrixEpoch(t *testing.T) {
	transition.SkipSlotCache.Disable()
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig()
	conf.BellatrixForkEpoch = 5
	params.OverrideBeaconConfig(conf)

	st, _ := util.DeterministicGenesisStateAltair(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	st, err := transition.ProcessSlots(context.Background(), st, params.BeaconConfig().SlotsPerEpoch*10)
	require.NoError(t, err)
	require.Equal(t, version.Bellatrix, st.Version())

	require.Equal(t, params.BeaconConfig().SlotsPerEpoch*10, st.Slot())
}

func TestProcessSlots_ThroughDenebEpoch(t *testing.T) {
	transition.SkipSlotCache.Disable()
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig()
	conf.DenebForkEpoch = 5
	params.OverrideBeaconConfig(conf)

	st, _ := util.DeterministicGenesisStateCapella(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	st, err := transition.ProcessSlots(context.Background(), st, params.BeaconConfig().SlotsPerEpoch*10)
	require.NoError(t, err)
	require.Equal(t, version.Deneb, st.Version())
	require.Equal(t, params.BeaconConfig().SlotsPerEpoch*10, st.Slot())
}

func TestProcessSlotsUsingNextSlotCache(t *testing.T) {
	s, _ := util.DeterministicGenesisState(t, 1)
	r := []byte{'a'}
	s, err := transition.ProcessSlotsUsingNextSlotCache(context.Background(), s, r, 5)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(5), s.Slot())
}

func TestProcessSlotsConditionally(t *testing.T) {
	ctx := context.Background()
	s, _ := util.DeterministicGenesisState(t, 1)

	t.Run("target slot below current slot", func(t *testing.T) {
		require.NoError(t, s.SetSlot(5))
		s, err := transition.ProcessSlotsIfPossible(ctx, s, 4)
		require.NoError(t, err)
		assert.Equal(t, primitives.Slot(5), s.Slot())
	})

	t.Run("target slot equal current slot", func(t *testing.T) {
		require.NoError(t, s.SetSlot(5))
		s, err := transition.ProcessSlotsIfPossible(ctx, s, 5)
		require.NoError(t, err)
		assert.Equal(t, primitives.Slot(5), s.Slot())
	})

	t.Run("target slot above current slot", func(t *testing.T) {
		require.NoError(t, s.SetSlot(5))
		s, err := transition.ProcessSlotsIfPossible(ctx, s, 6)
		require.NoError(t, err)
		assert.Equal(t, primitives.Slot(6), s.Slot())
	})
}
