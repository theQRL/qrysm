package util

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/time"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/crypto/bls"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	v1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/time/slots"
)

// GenerateFullBlockCapella generates a fully valid Capella block with the requested parameters.
// Use BlockGenConfig to declare the conditions you would like the block generated under.
// This function modifies the passed state as follows:

func GenerateFullBlockCapella(
	bState state.BeaconState,
	privs []bls.SecretKey,
	conf *BlockGenConfig,
	slot primitives.Slot,
) (*zondpb.SignedBeaconBlockCapella, error) {
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
	var pSlashings []*zondpb.ProposerSlashing
	numToGen := conf.NumProposerSlashings
	if numToGen > 0 {
		pSlashings, err = generateProposerSlashings(bState, privs, numToGen)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d proposer slashings:", numToGen)
		}
	}

	numToGen = conf.NumAttesterSlashings
	var aSlashings []*zondpb.AttesterSlashing
	if numToGen > 0 {
		aSlashings, err = generateAttesterSlashings(bState, privs, numToGen)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d attester slashings:", numToGen)
		}
	}

	numToGen = conf.NumAttestations
	var atts []*zondpb.Attestation
	if numToGen > 0 {
		atts, err = GenerateAttestations(bState, privs, numToGen, slot, false)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d attestations:", numToGen)
		}
	}

	numToGen = conf.NumDeposits
	var newDeposits []*zondpb.Deposit
	eth1Data := bState.Eth1Data()
	if numToGen > 0 {
		newDeposits, eth1Data, err = generateDepositsAndEth1Data(bState, numToGen)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d deposits:", numToGen)
		}
	}

	numToGen = conf.NumVoluntaryExits
	var exits []*zondpb.SignedVoluntaryExit
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
	newWithdrawals := make([]*v1.Withdrawal, 0)

	random, err := helpers.RandaoMix(bState, time.CurrentEpoch(bState))
	if err != nil {
		return nil, errors.Wrap(err, "could not process randao mix")
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

	parentExecution, err := stCopy.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, err
	}
	blockHash := indexToHash(uint64(slot))
	newExecutionPayloadCapella := &v1.ExecutionPayloadCapella{
		ParentHash:    parentExecution.BlockHash(),
		FeeRecipient:  make([]byte, 20),
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
		Withdrawals:   newWithdrawals,
	}
	var syncCommitteeBits []byte
	currSize := new(zondpb.SyncAggregate).SyncCommitteeBits.Len()
	switch currSize {
	case 512:
		syncCommitteeBits = bitfield.NewBitvector512()
	case 32:
		syncCommitteeBits = bitfield.NewBitvector32()
	default:
		return nil, errors.New("invalid bit vector size")
	}
	newSyncAggregate := &zondpb.SyncAggregate{
		SyncCommitteeBits:      syncCommitteeBits,
		SyncCommitteeSignature: append([]byte{0xC0}, make([]byte, 95)...),
	}

	newHeader := bState.LatestBlockHeader()
	prevStateRoot, err := bState.HashTreeRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not hash state")
	}
	newHeader.StateRoot = prevStateRoot[:]
	parentRoot, err := newHeader.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not hash the new header")
	}

	if slot == currentSlot {
		slot = currentSlot + 1
	}

	reveal, err := RandaoReveal(stCopy, time.CurrentEpoch(stCopy), privs)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute randao reveal")
	}

	idx, err := helpers.BeaconProposerIndex(ctx, stCopy)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute beacon proposer index")
	}

	changes := make([]*zondpb.SignedDilithiumToExecutionChange, conf.NumDilithiumChanges)
	for i := uint64(0); i < conf.NumDilithiumChanges; i++ {
		changes[i], err = GenerateDilithiumToExecutionChange(bState, privs[i+1], primitives.ValidatorIndex(i))
		if err != nil {
			return nil, err
		}
	}

	block := &zondpb.BeaconBlockCapella{
		Slot:          slot,
		ParentRoot:    parentRoot[:],
		ProposerIndex: idx,
		Body: &zondpb.BeaconBlockBodyCapella{
			Eth1Data:                    eth1Data,
			RandaoReveal:                reveal,
			ProposerSlashings:           pSlashings,
			AttesterSlashings:           aSlashings,
			Attestations:                atts,
			VoluntaryExits:              exits,
			Deposits:                    newDeposits,
			Graffiti:                    make([]byte, fieldparams.RootLength),
			SyncAggregate:               newSyncAggregate,
			ExecutionPayload:            newExecutionPayloadCapella,
			DilithiumToExecutionChanges: changes,
		},
	}

	// The fork can change after processing the state
	signature, err := BlockSignature(bState, block, privs)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block signature")
	}

	return &zondpb.SignedBeaconBlockCapella{Block: block, Signature: signature.Marshal()}, nil
}

// GenerateDilithiumToExecutionChange generates a valid dilithium to exec changes for validator `val` and its private key `priv` with the given beacon state `st`.
func GenerateDilithiumToExecutionChange(st state.BeaconState, priv bls.SecretKey, val primitives.ValidatorIndex) (*zondpb.SignedDilithiumToExecutionChange, error) {
	cred := indexToHash(uint64(val))
	pubkey := priv.PublicKey().Marshal()
	message := &zondpb.DilithiumToExecutionChange{
		ToExecutionAddress:  cred[12:],
		ValidatorIndex:      val,
		FromDilithiumPubkey: pubkey,
	}
	c := params.BeaconConfig()
	domain, err := signing.ComputeDomain(c.DomainDilithiumToExecutionChange, c.GenesisForkVersion, st.GenesisValidatorsRoot())
	if err != nil {
		return nil, err
	}
	sr, err := signing.ComputeSigningRoot(message, domain)
	if err != nil {
		return nil, err
	}
	signature := priv.Sign(sr[:]).Marshal()
	return &zondpb.SignedDilithiumToExecutionChange{
		Message:   message,
		Signature: signature,
	}, nil
}
