package blockchain

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	bitfield "github.com/theQRL/go-bitfield"
	"github.com/theQRL/go-qrl/crypto/pqcrypto"
	walletmldsa87 "github.com/theQRL/go-qrllib/wallet/ml_dsa_87"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/operations/slashings"
	"github.com/theQRL/qrysm/beacon-chain/operations/voluntaryexits"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	qrysmTime "github.com/theQRL/qrysm/time"
	"github.com/theQRL/qrysm/time/slots"
)

func TestReceiveBlock_Simulation(t *testing.T) {
	ctx := context.Background()

	numValidators := uint64(2625)
	numEpochs := primitives.Epoch(2)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	totalSlots := uint64(numEpochs) * uint64(slotsPerEpoch)
	t.Logf("SlotsPerEpoch: %d, totalSlots: %d", slotsPerEpoch, totalSlots)

	genesis, keys := util.DeterministicGenesisStateZond(t, numValidators)

	// Set genesis time far in the past so that any slot we test is in the past
	genesisTime := uint64(qrysmTime.Now().Unix()) - 90000000
	require.NoError(t, genesis.SetGenesisTime(genesisTime))

	s, req := minimalTestService(t,
		WithFinalizedStateAtStartUp(genesis),
		WithExitPool(voluntaryexits.NewPool()),
		WithSlashingPool(slashings.NewPool()),
	)
	s.SetGenesisTime(time.Unix(int64(genesisTime), 0))

	// Initialize the clock for the service
	var vr [32]byte
	copy(vr[:], genesis.GenesisValidatorsRoot())
	require.NoError(t, req.cs.SetClock(startup.NewClock(time.Unix(int64(genesisTime), 0), vr)))

	// We need to initialize the head and other things that minimalTestService might not fully do
	// minimalTestService calls NewService, but we might need to call saveGenesisData
	require.NoError(t, s.saveGenesisData(ctx, genesis))
	currState := genesis.Copy()
	addressToBalance := make(map[string]uint64)
	for slot := primitives.Slot(1); slot <= primitives.Slot(totalSlots); slot++ {
		// Prepare block generation config
		// Use default but maybe customize if needed.
		// Default includes 1 attestation.
		blockGenConf := util.DefaultBlockGenConfig()
		blockGenConf.FullSyncAggregate = true

		// Generate block
		signedBlock, err := util.GenerateFullBlockZond(currState, keys, blockGenConf, slot)
		require.NoError(t, err)

		roBlock, err := blocks.NewSignedBeaconBlock(signedBlock)
		require.NoError(t, err)

		blockRoot, err := roBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		// Call ReceiveBlock
		err = s.ReceiveBlock(ctx, roBlock, blockRoot)
		require.NoError(t, err, "Failed to receive block at slot %d", slot)

		// Verify head moved
		require.Equal(t, slot, s.head.block.Block().Slot())

		// Update currState for next block generation
		// In a real scenario, ReceiveBlock updates the service's head state.
		// We should use the post-state from the service.
		currState = s.head.state.Copy()
		executionData, err := roBlock.Block().Body().Execution()
		require.NoError(t, err)
		withdrawals, err := executionData.Withdrawals()
		require.NoError(t, err)
		for _, w := range withdrawals {
			addressToBalance[hex.EncodeToString(w.Address)] += w.Amount
		}
	}

	// Verify we reached the target epoch
	finalEpoch := slots.ToEpoch(s.head.block.Block().Slot())
	if uint64(finalEpoch) < uint64(numEpochs) {
		t.Errorf("Final epoch %d is less than expected %d", finalEpoch, numEpochs)
	}
	headState, err := s.HeadState(ctx)
	require.NoError(t, err)
	for i, bal := range headState.Balances() {
		require.Equal(t, true, bal >= genesis.Balances()[i],
			"Validator %d balance decreased: %d -> %d", i, genesis.Balances()[i], bal)
	}
	for i, val := range s.headState(ctx).Validators() {
		d, err := walletmldsa87.NewMLDSA87Descriptor()
		if err != nil {
			t.Fatal(err)
		}
		descriptor := d.ToDescriptor()
		withdrawalAddr, err := pqcrypto.PublicKeyAndDescriptorToAddress(val.PublicKey, descriptor)
		if err != nil {
			t.Fatal(err)
		}
		addressToBalance[hex.EncodeToString(withdrawalAddr.Bytes())] += headState.Balances()[i] - val.EffectiveBalance
	}
	total := uint64(0)
	i := 0
	for key, balance := range addressToBalance {
		i += 1
		t.Logf("%d %s %d", i, key, balance)
		total += balance
	}
	t.Logf("Total Emission: %d", total)
	expectedEmission := uint64(767287460522)
	require.Equal(t, expectedEmission, total, "Total emission mismatch")
}

func TestReceiveBlock_Simulation_MissedDuties(t *testing.T) {
	ctx := context.Background()

	numValidators := uint64(512)
	numEpochs := primitives.Epoch(10)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	totalSlots := uint64(numEpochs) * uint64(slotsPerEpoch)
	t.Logf("SlotsPerEpoch: %d, totalSlots: %d", slotsPerEpoch, totalSlots)

	genesis, keys := util.DeterministicGenesisStateZond(t, numValidators)

	// Set genesis time far in the past so that any slot we test is in the past
	genesisTime := uint64(qrysmTime.Now().Unix()) - 90000000
	require.NoError(t, genesis.SetGenesisTime(genesisTime))

	s, req := minimalTestService(t,
		WithFinalizedStateAtStartUp(genesis),
		WithExitPool(voluntaryexits.NewPool()),
		WithSlashingPool(slashings.NewPool()),
	)
	s.SetGenesisTime(time.Unix(int64(genesisTime), 0))

	// Initialize the clock for the service
	var vr [32]byte
	copy(vr[:], genesis.GenesisValidatorsRoot())
	require.NoError(t, req.cs.SetClock(startup.NewClock(time.Unix(int64(genesisTime), 0), vr)))

	require.NoError(t, s.saveGenesisData(ctx, genesis))
	currState := genesis.Copy()

	// Target validator index
	targetIdx := primitives.ValidatorIndex(16)
	val := genesis.Validators()[targetIdx]
	d, err := walletmldsa87.NewMLDSA87Descriptor()
	require.NoError(t, err)
	descriptor := d.ToDescriptor()
	targetAddr, err := pqcrypto.PublicKeyAndDescriptorToAddress(val.PublicKey, descriptor)
	require.NoError(t, err)
	t.Logf("Target validator address: %x", targetAddr)

	for slot := primitives.Slot(1); slot <= primitives.Slot(totalSlots); slot++ {
		// Process slots to find the proposer
		stCopy := currState.Copy()
		var err error
		stCopy, err = transition.ProcessSlots(ctx, stCopy, slot)
		require.NoError(t, err)

		proposerIdx, err := helpers.BeaconProposerIndex(ctx, stCopy)
		require.NoError(t, err)

		if proposerIdx == targetIdx {
			t.Logf("Slot %d: Target validator %d is proposer, skipping block", slot, targetIdx)
			currState = stCopy
			continue
		}

		// Prepare block generation config
		blockGenConf := util.DefaultBlockGenConfig()
		blockGenConf.FullSyncAggregate = true

		// Generate block
		signedBlock, err := util.GenerateFullBlockZond(currState, keys, blockGenConf, slot)
		require.NoError(t, err)

		// Filter SyncAggregate
		syncAggregate := signedBlock.Block.Body.SyncAggregate
		if syncAggregate != nil && len(syncAggregate.SyncCommitteeBits) > 0 {
			syncCommittee, err := stCopy.CurrentSyncCommittee()
			require.NoError(t, err)

			bits := syncAggregate.SyncCommitteeBits
			sigs := syncAggregate.SyncCommitteeSignatures

			newSigs := make([][]byte, 0, len(sigs))
			sigIdx := 0

			for i, p := range syncCommittee.Pubkeys {
				idx, ok := stCopy.ValidatorIndexByPubkey(bytesutil.ToBytes2592(p))

				bitSet := false
				switch len(bits) {
				case 64: // 512 bits
					bitSet = bitfield.Bitvector512(bits).BitAt(uint64(i))
				case 16: // 128 bits
					bitSet = bitfield.Bitvector128(bits).BitAt(uint64(i))
				case 4: // 32 bits
					bitSet = bitfield.Bitvector32(bits).BitAt(uint64(i))
				case 2: // 16 bits
					bitSet = bitfield.Bitvector16(bits).BitAt(uint64(i))
				}

				if bitSet {
					if ok && idx == targetIdx {
						//t.Logf("Slot %d: Target validator %d is sync committee, skipping sync committee", slot, targetIdx)
						// Unset the bit
						switch len(bits) {
						case 64:
							bitfield.Bitvector512(bits).SetBitAt(uint64(i), false)
						case 16:
							bitfield.Bitvector128(bits).SetBitAt(uint64(i), false)
						case 4:
							bitfield.Bitvector32(bits).SetBitAt(uint64(i), false)
						case 2:
							bitfield.Bitvector16(bits).SetBitAt(uint64(i), false)
						}
						// Skip this signature
					} else {
						newSigs = append(newSigs, sigs[sigIdx])
					}
					sigIdx++
				}
			}
			syncAggregate.SyncCommitteeSignatures = newSigs
		}

		// Filter Attestations
		newAttestations := make([]*qrysmpb.Attestation, 0, len(signedBlock.Block.Body.Attestations))
		for _, att := range signedBlock.Block.Body.Attestations {
			committee, err := helpers.BeaconCommitteeFromState(ctx, stCopy, att.Data.Slot, att.Data.CommitteeIndex)
			require.NoError(t, err)

			bits := bitfield.Bitlist(att.AggregationBits)
			sigs := att.Signatures

			newSigs := make([][]byte, 0, len(sigs))
			sigIdx := 0
			for i, vIdx := range committee {
				if bits.BitAt(uint64(i)) {
					if vIdx == targetIdx {
						t.Logf("Slot %d: Target validator %d is attester, skipping attestation", slot, vIdx)
						bits.SetBitAt(uint64(i), false)
						// Skip signature
					} else {
						newSigs = append(newSigs, sigs[sigIdx])
					}
					sigIdx++
				}
			}
			if len(newSigs) > 0 {
				att.AggregationBits = []byte(bits)
				att.Signatures = newSigs
				newAttestations = append(newAttestations, att)
			}
		}
		signedBlock.Block.Body.Attestations = newAttestations

		// Recalculate block signature
		sig, err := util.BlockSignature(currState, signedBlock.Block, keys)
		require.NoError(t, err)
		signedBlock.Signature = sig.Marshal()

		roBlock, err := blocks.NewSignedBeaconBlock(signedBlock)
		require.NoError(t, err)

		blockRoot, err := roBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		// Call ReceiveBlock
		err = s.ReceiveBlock(ctx, roBlock, blockRoot)
		require.NoError(t, err, "Failed to receive block at slot %d", slot)

		// Update currState for next block generation
		currState = s.head.state.Copy()
	}

	headState, err := s.HeadState(ctx)
	require.NoError(t, err)

	targetBalance := headState.Balances()[targetIdx]
	otherBalance := headState.Balances()[1]

	require.Equal(t, uint64(39996066588426), targetBalance, "Target validator balance mismatch")
	require.Equal(t, uint64(40000484814226), otherBalance, "Other validator balance mismatch")
}

func TestReceiveBlock_Simulation_MissedDuties_WithLeak(t *testing.T) {
	ctx := context.Background()

	numValidators := uint64(2048)
	numEpochs := primitives.Epoch(10)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	totalSlots := uint64(numEpochs) * uint64(slotsPerEpoch)
	t.Logf("SlotsPerEpoch: %d, totalSlots: %d", slotsPerEpoch, totalSlots)

	genesis, keys := util.DeterministicGenesisStateZond(t, numValidators)

	// Set genesis time far in the past so that any slot we test is in the past
	genesisTime := uint64(qrysmTime.Now().Unix()) - 90000000
	require.NoError(t, genesis.SetGenesisTime(genesisTime))

	s, req := minimalTestService(t,
		WithFinalizedStateAtStartUp(genesis),
		WithExitPool(voluntaryexits.NewPool()),
		WithSlashingPool(slashings.NewPool()),
	)
	s.SetGenesisTime(time.Unix(int64(genesisTime), 0))

	// Initialize the clock for the service
	var vr [32]byte
	copy(vr[:], genesis.GenesisValidatorsRoot())
	require.NoError(t, req.cs.SetClock(startup.NewClock(time.Unix(int64(genesisTime), 0), vr)))

	require.NoError(t, s.saveGenesisData(ctx, genesis))
	currState := genesis.Copy()

	// Target validator index
	targetIdx := primitives.ValidatorIndex(16)
	val := genesis.Validators()[targetIdx]
	d, err := walletmldsa87.NewMLDSA87Descriptor()
	require.NoError(t, err)
	descriptor := d.ToDescriptor()
	targetAddr, err := pqcrypto.PublicKeyAndDescriptorToAddress(val.PublicKey, descriptor)
	require.NoError(t, err)
	t.Logf("Target validator address: %x", targetAddr)

	for slot := primitives.Slot(1); slot <= primitives.Slot(totalSlots); slot++ {
		// Process slots to find the proposer
		stCopy := currState.Copy()
		var err error
		stCopy, err = transition.ProcessSlots(ctx, stCopy, slot)
		require.NoError(t, err)

		proposerIdx, err := helpers.BeaconProposerIndex(ctx, stCopy)
		require.NoError(t, err)

		if proposerIdx == targetIdx {
			t.Logf("Slot %d: Target validator %d is proposer, skipping block", slot, targetIdx)
			currState = stCopy
			continue
		}

		// Prepare block generation config
		blockGenConf := util.DefaultBlockGenConfig()
		blockGenConf.FullSyncAggregate = true

		// Generate block
		signedBlock, err := util.GenerateFullBlockZond(currState, keys, blockGenConf, slot)
		require.NoError(t, err)

		// Filter SyncAggregate
		syncAggregate := signedBlock.Block.Body.SyncAggregate
		if syncAggregate != nil && len(syncAggregate.SyncCommitteeBits) > 0 {
			syncCommittee, err := stCopy.CurrentSyncCommittee()
			require.NoError(t, err)

			bits := syncAggregate.SyncCommitteeBits
			sigs := syncAggregate.SyncCommitteeSignatures

			newSigs := make([][]byte, 0, len(sigs))
			sigIdx := 0

			for i, p := range syncCommittee.Pubkeys {
				idx, ok := stCopy.ValidatorIndexByPubkey(bytesutil.ToBytes2592(p))

				bitSet := false
				switch len(bits) {
				case 64: // 512 bits
					bitSet = bitfield.Bitvector512(bits).BitAt(uint64(i))
				case 16: // 128 bits
					bitSet = bitfield.Bitvector128(bits).BitAt(uint64(i))
				case 4: // 32 bits
					bitSet = bitfield.Bitvector32(bits).BitAt(uint64(i))
				case 2: // 16 bits
					bitSet = bitfield.Bitvector16(bits).BitAt(uint64(i))
				}

				if bitSet {
					if ok && idx == targetIdx {
						//t.Logf("Slot %d: Target validator %d is sync committee, skipping sync committee", slot, targetIdx)
						// Unset the bit
						switch len(bits) {
						case 64:
							bitfield.Bitvector512(bits).SetBitAt(uint64(i), false)
						case 16:
							bitfield.Bitvector128(bits).SetBitAt(uint64(i), false)
						case 4:
							bitfield.Bitvector32(bits).SetBitAt(uint64(i), false)
						case 2:
							bitfield.Bitvector16(bits).SetBitAt(uint64(i), false)
						}
						// Skip this signature
					} else {
						newSigs = append(newSigs, sigs[sigIdx])
					}
					sigIdx++
				}
			}
			syncAggregate.SyncCommitteeSignatures = newSigs
		}

		// Filter Attestations
		newAttestations := make([]*qrysmpb.Attestation, 0, len(signedBlock.Block.Body.Attestations))
		for _, att := range signedBlock.Block.Body.Attestations {
			committee, err := helpers.BeaconCommitteeFromState(ctx, stCopy, att.Data.Slot, att.Data.CommitteeIndex)
			require.NoError(t, err)

			bits := bitfield.Bitlist(att.AggregationBits)
			sigs := att.Signatures

			newSigs := make([][]byte, 0, len(sigs))
			sigIdx := 0
			for i, vIdx := range committee {
				if bits.BitAt(uint64(i)) {
					if currState.Balances()[targetIdx] > 0 && (vIdx == targetIdx || uint64(i) >= bits.Count()/2) {
						//t.Logf("Slot %d: Target validator %d is attester, skipping attestation", slot, vIdx)
						bits.SetBitAt(uint64(i), false)
						// Skip signature
					} else {
						newSigs = append(newSigs, sigs[sigIdx])
					}
					sigIdx++
				}
			}
			if len(newSigs) > 0 {
				att.AggregationBits = []byte(bits)
				att.Signatures = newSigs
				newAttestations = append(newAttestations, att)
			}
		}
		signedBlock.Block.Body.Attestations = newAttestations

		// Recalculate block signature
		sig, err := util.BlockSignature(currState, signedBlock.Block, keys)
		require.NoError(t, err)
		signedBlock.Signature = sig.Marshal()

		roBlock, err := blocks.NewSignedBeaconBlock(signedBlock)
		require.NoError(t, err)

		blockRoot, err := roBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		// Call ReceiveBlock
		err = s.ReceiveBlock(ctx, roBlock, blockRoot)
		require.NoError(t, err, "Failed to receive block at slot %d", slot)

		// Update currState for next block generation
		currState = s.head.state.Copy()
	}

	headState, err := s.HeadState(ctx)
	require.NoError(t, err)

	targetBalance := headState.Balances()[targetIdx]
	otherBalance := headState.Balances()[1]

	require.Equal(t, uint64(39990857626551), targetBalance, "Target validator balance mismatch")
	require.Equal(t, uint64(39999536080860), otherBalance, "Other validator balance mismatch")
}

func TestReceiveBlock_Simulation_ProposerSlashing(t *testing.T) {
	ctx := context.Background()

	numValidators := uint64(512)
	numEpochs := primitives.Epoch(4)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	totalSlots := uint64(numEpochs) * uint64(slotsPerEpoch)
	t.Logf("SlotsPerEpoch: %d, totalSlots: %d", slotsPerEpoch, totalSlots)

	genesis, keys := util.DeterministicGenesisStateZond(t, numValidators)

	// Set genesis time far in the past so that any slot we test is in the past
	genesisTime := uint64(qrysmTime.Now().Unix()) - 90000000
	require.NoError(t, genesis.SetGenesisTime(genesisTime))

	s, req := minimalTestService(t,
		WithFinalizedStateAtStartUp(genesis),
		WithExitPool(voluntaryexits.NewPool()),
		WithSlashingPool(slashings.NewPool()),
	)
	s.SetGenesisTime(time.Unix(int64(genesisTime), 0))

	// Initialize the clock for the service
	var vr [32]byte
	copy(vr[:], genesis.GenesisValidatorsRoot())
	require.NoError(t, req.cs.SetClock(startup.NewClock(time.Unix(int64(genesisTime), 0), vr)))

	require.NoError(t, s.saveGenesisData(ctx, genesis))
	currState := genesis.Copy()

	// Target validator index to be slashed
	targetIdx := primitives.ValidatorIndex(16)
	val := genesis.Validators()[targetIdx]
	initialEffectiveBalance := val.EffectiveBalance
	t.Logf("Target validator public key: %x", val.PublicKey)
	t.Logf("Initial EffectiveBalance: %d", initialEffectiveBalance)

	slashingIncluded := false
	slashedInSlot := primitives.Slot(0)

	for slot := primitives.Slot(1); slot <= primitives.Slot(totalSlots); slot++ {
		// Process slots to find the proposer
		stCopy := currState.Copy()
		var err error
		stCopy, err = transition.ProcessSlots(ctx, stCopy, slot)
		require.NoError(t, err)

		proposerIdx, err := helpers.BeaconProposerIndex(ctx, stCopy)
		require.NoError(t, err)

		vProposer, err := stCopy.ValidatorAtIndex(proposerIdx)
		require.NoError(t, err)

		if vProposer.Slashed {
			t.Logf("Slot %d: Proposer %d is slashed, skipping block", slot, proposerIdx)
			currState = stCopy
			continue
		}

		// Prepare block generation config
		blockGenConf := util.DefaultBlockGenConfig()
		blockGenConf.FullSyncAggregate = true

		// Generate block
		signedBlock, err := util.GenerateFullBlockZond(currState, keys, blockGenConf, slot)
		require.NoError(t, err)

		if !slashingIncluded && slot > slotsPerEpoch {
			// Generate a proposer slashing for the target validator at an earlier slot
			slashingSlot := primitives.Slot(1)
			stAtSlashingSlot := genesis.Copy()
			stAtSlashingSlot, err = transition.ProcessSlots(ctx, stAtSlashingSlot, slashingSlot)
			require.NoError(t, err)

			slashing, err := util.GenerateProposerSlashingForValidator(stAtSlashingSlot, keys[targetIdx], targetIdx)
			require.NoError(t, err)

			signedBlock.Block.Body.ProposerSlashings = append(signedBlock.Block.Body.ProposerSlashings, slashing)
			slashingIncluded = true
			slashedInSlot = slot

			// Recalculate block signature
			sig, err := util.BlockSignature(currState, signedBlock.Block, keys)
			require.NoError(t, err)
			signedBlock.Signature = sig.Marshal()
		}

		roBlock, err := blocks.NewSignedBeaconBlock(signedBlock)
		require.NoError(t, err)

		blockRoot, err := roBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		// Call ReceiveBlock
		err = s.ReceiveBlock(ctx, roBlock, blockRoot)
		require.NoError(t, err, "Failed to receive block at slot %d", slot)

		// Update currState for next block generation
		currState = s.head.state.Copy()
		if slashingIncluded {
			v, err := currState.ValidatorAtIndex(targetIdx)
			require.NoError(t, err)
			if v.Slashed {
				// If we have moved past the epoch in which slashing was included,
				// the effective balance should have been updated.
				if slots.ToEpoch(slot) > slots.ToEpoch(slashedInSlot) {
					if v.EffectiveBalance < initialEffectiveBalance {
						t.Logf("Slot %d: Target validator %d slashed and EffectiveBalance successfully decreased to %d", slot, targetIdx, v.EffectiveBalance)
						break
					}
				}
			}
		}
	}

	headState, err := s.HeadState(ctx)
	require.NoError(t, err)

	targetVal, err := headState.ValidatorAtIndex(targetIdx)
	require.NoError(t, err)

	require.Equal(t, true, targetVal.Slashed, "Target validator should be slashed")
	require.Equal(t, uint64(38749000000000), targetVal.EffectiveBalance, "Target effective balance mismatch")
	require.Equal(t, uint64(38749713240973), headState.Balances()[targetIdx], "Target validator balance mismatch")
}

func TestReceiveBlock_Simulation_AttesterSlashing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long simulation in short mode")
	}

	tests := []struct {
		name          string
		numValidators uint64
		slashingSlot  primitives.Slot
		expectedEff   uint64 // Expected EffectiveBalance after all penalties.
	}{
		{
			name:          "512 validators, slashing at slot 129",
			numValidators: 512,
			slashingSlot:  129,
			expectedEff:   38749000000000,
		},
		{
			name:          "256 validators, slashing at slot 257",
			numValidators: 256,
			slashingSlot:  257,
			expectedEff:   38750000000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
			// Run for enough epochs to reach the slashing slot and see the effective balance update
			numEpochs := slots.ToEpoch(tt.slashingSlot) + 3
			totalSlots := uint64(numEpochs) * uint64(slotsPerEpoch)

			genesis, keys := util.DeterministicGenesisStateZond(t, tt.numValidators)
			genesisTime := uint64(qrysmTime.Now().Unix()) - 90000000
			require.NoError(t, genesis.SetGenesisTime(genesisTime))

			s, req := minimalTestService(t,
				WithFinalizedStateAtStartUp(genesis),
				WithExitPool(voluntaryexits.NewPool()),
				WithSlashingPool(slashings.NewPool()),
			)
			s.SetGenesisTime(time.Unix(int64(genesisTime), 0))

			var vr [32]byte
			copy(vr[:], genesis.GenesisValidatorsRoot())
			require.NoError(t, req.cs.SetClock(startup.NewClock(time.Unix(int64(genesisTime), 0), vr)))
			require.NoError(t, s.saveGenesisData(ctx, genesis))

			currState := genesis.Copy()
			targetIdx := primitives.ValidatorIndex(17)
			val := genesis.Validators()[targetIdx]
			initialEffectiveBalance := val.EffectiveBalance

			slashingIncluded := false
			slashedInSlot := primitives.Slot(0)
			var proposerIdxAtSlashing primitives.ValidatorIndex
			var proposerBalanceBeforeSlashing uint64

			for slot := primitives.Slot(1); slot <= primitives.Slot(totalSlots); slot++ {
				stCopy := currState.Copy()
				stCopy, err := transition.ProcessSlots(ctx, stCopy, slot)
				require.NoError(t, err)

				proposerIdx, err := helpers.BeaconProposerIndex(ctx, stCopy)
				require.NoError(t, err)

				vProposer, err := stCopy.ValidatorAtIndex(proposerIdx)
				require.NoError(t, err)

				if vProposer.Slashed {
					currState = stCopy
					continue
				}

				blockGenConf := util.DefaultBlockGenConfig()
				blockGenConf.FullSyncAggregate = true
				signedBlock, err := util.GenerateFullBlockZond(currState, keys, blockGenConf, slot)
				require.NoError(t, err)

				if !slashingIncluded && slot == tt.slashingSlot {
					// Prepare slashing data using state from a previous slot
					stAtSlashingSlot := genesis.Copy()
					stAtSlashingSlot, err = transition.ProcessSlots(ctx, stAtSlashingSlot, 1)
					require.NoError(t, err)

					slashing, err := util.GenerateAttesterSlashingForValidator(stAtSlashingSlot, keys[targetIdx], targetIdx)
					require.NoError(t, err)

					signedBlock.Block.Body.AttesterSlashings = append(signedBlock.Block.Body.AttesterSlashings, slashing)
					slashingIncluded = true
					slashedInSlot = slot
					proposerIdxAtSlashing = proposerIdx
					proposerBalanceBeforeSlashing = s.head.state.Balances()[proposerIdx]

					sig, err := util.BlockSignature(currState, signedBlock.Block, keys)
					require.NoError(t, err)
					signedBlock.Signature = sig.Marshal()
				}

				roBlock, err := blocks.NewSignedBeaconBlock(signedBlock)
				require.NoError(t, err)
				blockRoot, err := roBlock.Block().HashTreeRoot()
				require.NoError(t, err)

				err = s.ReceiveBlock(ctx, roBlock, blockRoot)
				require.NoError(t, err, "Failed to receive block at slot %d", slot)

				currState = s.head.state.Copy()
				targetVal, _ := currState.ValidatorAtIndex(targetIdx)

				if slashingIncluded {
					// 1. Verify expected slot of slashing
					if slot == slashedInSlot {
						if !targetVal.Slashed {
							t.Fatalf("Validator %d should be marked as slashed in slot %d", targetIdx, slot)
						}
						// Verify whistleblower reward for proposer
						reward := initialEffectiveBalance / params.BeaconConfig().WhistleBlowerRewardQuotient
						if currState.Balances()[proposerIdxAtSlashing] < proposerBalanceBeforeSlashing+reward {
							t.Errorf("Proposer did not receive expected whistleblower reward")
						}
					}

					// 2. Verify expected effective balance after epoch transition
					if slots.ToEpoch(slot) > slots.ToEpoch(slashedInSlot) {
						if targetVal.EffectiveBalance < initialEffectiveBalance {
							require.Equal(t, tt.expectedEff, targetVal.EffectiveBalance)
							t.Logf("Slot %d: Target validator %d slashed and EffectiveBalance successfully decreased to %d",
								slot, targetIdx, targetVal.EffectiveBalance)
							return // Subtest successful
						}
					}
				}
			}
			t.Errorf("Slashing effects for %s not fully observed", tt.name)
		})
	}
}
