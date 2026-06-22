package util

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/hash"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	v1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
)

// GenerateFullBlockZond generates a fully valid Zond block with the requested parameters.
// Use BlockGenConfig to declare the conditions you would like the block generated under.
// This function modifies the passed state as follows:
func GenerateFullBlockZond(
	bState state.BeaconState,
	privs []ml_dsa_87.MLDSA87Key,
	conf *BlockGenConfig,
	slot primitives.Slot,
) (*qrysmpb.SignedBeaconBlockZond, error) {
	ctx := context.Background()
	currentSlot := bState.Slot()
	if currentSlot > slot {
		return nil, fmt.Errorf("current slot in state is larger than given slot. %d > %d", currentSlot, slot)
	}
	bState = bState.Copy()

	if conf == nil {
		conf = &BlockGenConfig{}
	}

	var err error
	var pSlashings []*qrysmpb.ProposerSlashing
	numToGen := conf.NumProposerSlashings
	if numToGen > 0 {
		pSlashings, err = generateProposerSlashings(bState, privs, numToGen)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d proposer slashings:", numToGen)
		}
	}

	numToGen = conf.NumAttesterSlashings
	var aSlashings []*qrysmpb.AttesterSlashing
	if numToGen > 0 {
		aSlashings, err = generateAttesterSlashings(bState, privs, numToGen)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d attester slashings:", numToGen)
		}
	}

	numToGen = conf.NumAttestations
	var atts []*qrysmpb.Attestation
	if numToGen > 0 {
		atts, err = GenerateAttestations(bState, privs, numToGen, slot, false)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d attestations:", numToGen)
		}
	}

	numToGen = conf.NumDeposits
	var newDeposits []*qrysmpb.Deposit
	executionData := bState.ExecutionData()
	if numToGen > 0 {
		newDeposits, executionData, err = generateDepositsAndExecutionData(bState, numToGen)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d deposits:", numToGen)
		}
	}

	numToGen = conf.NumVoluntaryExits
	var exits []*qrysmpb.SignedVoluntaryExit
	if numToGen > 0 {
		exits, err = generateVoluntaryExits(bState, privs, numToGen)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d attester slashings:", numToGen)
		}
	}

	numToGen = conf.NumTransactions
	newTransactions := make([][]byte, numToGen)
	for i := uint64(0); i < numToGen; i++ {
		newTransactions[i] = bytesutil.Uint64ToBytesLittleEndian(i)
	}

	random, err := helpers.RandaoMix(bState, time.CurrentEpoch(bState))
	if err != nil {
		return nil, errors.Wrap(err, "could not process randao mix")
	}

	if slot == currentSlot {
		slot = currentSlot + 1
	}

	timestamp, err := slots.ToTime(bState.GenesisTime(), slot)
	if err != nil {
		return nil, errors.Wrap(err, "could not get current timestamp")
	}

	stCopy := bState.Copy()
	stCopy, err = transition.ProcessSlots(context.Background(), stCopy, slot)
	if err != nil {
		return nil, err
	}

	withdrawals, err := stCopy.ExpectedWithdrawals()
	if err != nil {
		return nil, errors.Wrapf(err, "failed generating %d withdrawals:", numToGen)
	}

	parentExecution, err := stCopy.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, err
	}
	blockHash := indexToHash(uint64(slot))
	newExecutionPayloadZond := &v1.ExecutionPayloadZond{
		ParentHash:    parentExecution.BlockHash(),
		FeeRecipient:  make([]byte, 64),
		StateRoot:     params.BeaconConfig().ZeroHash[:],
		ReceiptsRoot:  params.BeaconConfig().ZeroHash[:],
		LogsBloom:     make([]byte, 256),
		PrevRandao:    random,
		BlockNumber:   uint64(slot),
		ExtraData:     params.BeaconConfig().ZeroHash[:],
		BaseFeePerGas: params.BeaconConfig().ZeroHash[:],
		BlockHash:     blockHash[:],
		Timestamp:     uint64(timestamp.Unix()),
		Transactions:  newTransactions,
		Withdrawals:   withdrawals,
	}

	newHeader := bState.LatestBlockHeader()
	// If it's already set, it means the state was advanced through skipped slots,
	// and the header already contains the correct state root for the parent block.
	if bytes.Equal(newHeader.StateRoot, params.BeaconConfig().ZeroHash[:]) {
		prevStateRoot, err := bState.HashTreeRoot(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "could not hash state")
		}
		newHeader.StateRoot = prevStateRoot[:]
	}
	parentRoot, err := newHeader.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not hash the new header")
	}

	var newSyncAggregate *qrysmpb.SyncAggregate
	if conf.FullSyncAggregate {
		newSyncAggregate, err = generateSyncAggregate(bState, privs, parentRoot)
		if err != nil {
			return nil, errors.Wrap(err, "failed generating syncAggregate")
		}
	} else {
		var syncCommitteeBits []byte
		currSize := new(qrysmpb.SyncAggregate).SyncCommitteeBits.Len()
		switch currSize {
		case 512:
			syncCommitteeBits = bitfield.NewBitvector512()
		case 128:
			syncCommitteeBits = bitfield.NewBitvector128()
		case 32:
			syncCommitteeBits = bitfield.NewBitvector32()
		case 16:
			syncCommitteeBits = bitfield.NewBitvector16()
		default:
			return nil, errors.New("invalid bit vector size")
		}
		newSyncAggregate = &qrysmpb.SyncAggregate{
			SyncCommitteeBits:       syncCommitteeBits,
			SyncCommitteeSignatures: [][]byte{},
		}
	}

	reveal, err := RandaoReveal(stCopy, time.CurrentEpoch(stCopy), privs)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute randao reveal")
	}

	idx, err := helpers.BeaconProposerIndex(ctx, stCopy)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute beacon proposer index")
	}

	block := &qrysmpb.BeaconBlockZond{
		Slot:          slot,
		ParentRoot:    parentRoot[:],
		ProposerIndex: idx,
		Body: &qrysmpb.BeaconBlockBodyZond{
			ExecutionData:     executionData,
			RandaoReveal:      reveal,
			ProposerSlashings: pSlashings,
			AttesterSlashings: aSlashings,
			Attestations:      atts,
			VoluntaryExits:    exits,
			Deposits:          newDeposits,
			Graffiti:          make([]byte, fieldparams.RootLength),
			SyncAggregate:     newSyncAggregate,
			ExecutionPayload:  newExecutionPayloadZond,
		},
	}

	// The fork can change after processing the state
	signature, err := BlockSignature(bState, block, privs)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block signature")
	}

	return &qrysmpb.SignedBeaconBlockZond{Block: block, Signature: signature.Marshal()}, nil
}

func indexToHash(i uint64) [32]byte {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], i)
	return hash.Hash(b[:])
}
