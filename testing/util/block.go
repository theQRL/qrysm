package util

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/time"
	"github.com/theQRL/qrysm/v4/beacon-chain/db/iface"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/crypto/bls"
	"github.com/theQRL/qrysm/v4/crypto/rand"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	v1 "github.com/theQRL/qrysm/v4/proto/zond/v1"
	v2 "github.com/theQRL/qrysm/v4/proto/zond/v2"
	"github.com/theQRL/qrysm/v4/testing/assertions"
	"github.com/theQRL/qrysm/v4/testing/require"
)

// BlockGenConfig is used to define the requested conditions
// for block generation.
type BlockGenConfig struct {
	NumProposerSlashings uint64
	NumAttesterSlashings uint64
	NumAttestations      uint64
	NumDeposits          uint64
	NumVoluntaryExits    uint64
	NumTransactions      uint64 // Only for post Bellatrix blocks
	FullSyncAggregate    bool
	NumDilithiumChanges  uint64 // Only for post Capella blocks
}

// DefaultBlockGenConfig returns the block config that utilizes the
// current params in the beacon config.
func DefaultBlockGenConfig() *BlockGenConfig {
	return &BlockGenConfig{
		NumProposerSlashings: 0,
		NumAttesterSlashings: 0,
		NumAttestations:      1,
		NumDeposits:          0,
		NumVoluntaryExits:    0,
		NumTransactions:      0,
		NumDilithiumChanges:  0,
	}
}

// NewBeaconBlock creates a beacon block with minimum marshalable fields.
func NewBeaconBlock() *zondpb.SignedBeaconBlock {
	return &zondpb.SignedBeaconBlock{
		Block: &zondpb.BeaconBlock{
			ParentRoot: make([]byte, fieldparams.RootLength),
			StateRoot:  make([]byte, fieldparams.RootLength),
			Body: &zondpb.BeaconBlockBody{
				RandaoReveal: make([]byte, dilithium2.CryptoBytes),
				Eth1Data: &zondpb.Eth1Data{
					DepositRoot: make([]byte, fieldparams.RootLength),
					BlockHash:   make([]byte, fieldparams.RootLength),
				},
				Graffiti:          make([]byte, fieldparams.RootLength),
				Attestations:      []*zondpb.Attestation{},
				AttesterSlashings: []*zondpb.AttesterSlashing{},
				Deposits:          []*zondpb.Deposit{},
				ProposerSlashings: []*zondpb.ProposerSlashing{},
				VoluntaryExits:    []*zondpb.SignedVoluntaryExit{},
			},
		},
		Signature: make([]byte, dilithium2.CryptoBytes),
	}
}

func NewBlobsidecar() *zondpb.SignedBlobSidecar {
	return &zondpb.SignedBlobSidecar{
		Message: &zondpb.BlobSidecar{
			BlockRoot:       make([]byte, fieldparams.RootLength),
			BlockParentRoot: make([]byte, fieldparams.RootLength),
			Blob:            make([]byte, fieldparams.BlobLength),
			KzgCommitment:   make([]byte, dilithium2.CryptoPublicKeyBytes),
			KzgProof:        make([]byte, dilithium2.CryptoPublicKeyBytes),
		},
		Signature: make([]byte, dilithium2.CryptoBytes),
	}
}

// GenerateFullBlock generates a fully valid block with the requested parameters.
// Use BlockGenConfig to declare the conditions you would like the block generated under.
func GenerateFullBlock(
	bState state.BeaconState,
	privs []bls.SecretKey,
	conf *BlockGenConfig,
	slot primitives.Slot,
) (*zondpb.SignedBeaconBlock, error) {
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
			return nil, errors.Wrapf(err, "failed generating %d voluntary exits:", numToGen)
		}
	}

	newHeader := bState.LatestBlockHeader()
	prevStateRoot, err := bState.HashTreeRoot(ctx)
	if err != nil {
		return nil, err
	}
	newHeader.StateRoot = prevStateRoot[:]
	parentRoot, err := newHeader.HashTreeRoot()
	if err != nil {
		return nil, err
	}

	if slot == currentSlot {
		slot = currentSlot + 1
	}

	// Temporarily incrementing the beacon state slot here since BeaconProposerIndex is a
	// function deterministic on beacon state slot.
	if err := bState.SetSlot(slot); err != nil {
		return nil, err
	}
	reveal, err := RandaoReveal(bState, time.CurrentEpoch(bState), privs)
	if err != nil {
		return nil, err
	}

	idx, err := helpers.BeaconProposerIndex(ctx, bState)
	if err != nil {
		return nil, err
	}

	block := &zondpb.BeaconBlock{
		Slot:          slot,
		ParentRoot:    parentRoot[:],
		ProposerIndex: idx,
		Body: &zondpb.BeaconBlockBody{
			Eth1Data:          eth1Data,
			RandaoReveal:      reveal,
			ProposerSlashings: pSlashings,
			AttesterSlashings: aSlashings,
			Attestations:      atts,
			VoluntaryExits:    exits,
			Deposits:          newDeposits,
			Graffiti:          make([]byte, fieldparams.RootLength),
		},
	}
	if err := bState.SetSlot(currentSlot); err != nil {
		return nil, err
	}

	signature, err := BlockSignature(bState, block, privs)
	if err != nil {
		return nil, err
	}

	return &zondpb.SignedBeaconBlock{Block: block, Signature: signature.Marshal()}, nil
}

// GenerateProposerSlashingForValidator for a specific validator index.
func GenerateProposerSlashingForValidator(
	bState state.BeaconState,
	priv bls.SecretKey,
	idx primitives.ValidatorIndex,
) (*zondpb.ProposerSlashing, error) {
	header1 := HydrateSignedBeaconHeader(&zondpb.SignedBeaconBlockHeader{
		Header: &zondpb.BeaconBlockHeader{
			ProposerIndex: idx,
			Slot:          bState.Slot(),
			BodyRoot:      bytesutil.PadTo([]byte{0, 1, 0}, fieldparams.RootLength),
		},
	})
	currentEpoch := time.CurrentEpoch(bState)
	var err error
	header1.Signature, err = signing.ComputeDomainAndSign(bState, currentEpoch, header1.Header, params.BeaconConfig().DomainBeaconProposer, priv)
	if err != nil {
		return nil, err
	}

	header2 := &zondpb.SignedBeaconBlockHeader{
		Header: &zondpb.BeaconBlockHeader{
			ProposerIndex: idx,
			Slot:          bState.Slot(),
			BodyRoot:      bytesutil.PadTo([]byte{0, 2, 0}, fieldparams.RootLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ParentRoot:    make([]byte, fieldparams.RootLength),
		},
	}
	header2.Signature, err = signing.ComputeDomainAndSign(bState, currentEpoch, header2.Header, params.BeaconConfig().DomainBeaconProposer, priv)
	if err != nil {
		return nil, err
	}

	return &zondpb.ProposerSlashing{
		Header_1: header1,
		Header_2: header2,
	}, nil
}

func generateProposerSlashings(
	bState state.BeaconState,
	privs []bls.SecretKey,
	numSlashings uint64,
) ([]*zondpb.ProposerSlashing, error) {
	proposerSlashings := make([]*zondpb.ProposerSlashing, numSlashings)
	for i := uint64(0); i < numSlashings; i++ {
		proposerIndex, err := randValIndex(bState)
		if err != nil {
			return nil, err
		}
		slashing, err := GenerateProposerSlashingForValidator(bState, privs[proposerIndex], proposerIndex)
		if err != nil {
			return nil, err
		}
		proposerSlashings[i] = slashing
	}
	return proposerSlashings, nil
}

// GenerateAttesterSlashingForValidator for a specific validator index.
func GenerateAttesterSlashingForValidator(
	bState state.BeaconState,
	priv bls.SecretKey,
	idx primitives.ValidatorIndex,
) (*zondpb.AttesterSlashing, error) {
	currentEpoch := time.CurrentEpoch(bState)

	att1 := &zondpb.IndexedAttestation{
		Data: &zondpb.AttestationData{
			Slot:            bState.Slot(),
			CommitteeIndex:  0,
			BeaconBlockRoot: make([]byte, fieldparams.RootLength),
			Target: &zondpb.Checkpoint{
				Epoch: currentEpoch,
				Root:  params.BeaconConfig().ZeroHash[:],
			},
			Source: &zondpb.Checkpoint{
				Epoch: currentEpoch + 1,
				Root:  params.BeaconConfig().ZeroHash[:],
			},
		},
		AttestingIndices: []uint64{uint64(idx)},
	}
	var err error
	att1.Signature, err = signing.ComputeDomainAndSign(bState, currentEpoch, att1.Data, params.BeaconConfig().DomainBeaconAttester, priv)
	if err != nil {
		return nil, err
	}

	att2 := &zondpb.IndexedAttestation{
		Data: &zondpb.AttestationData{
			Slot:            bState.Slot(),
			CommitteeIndex:  0,
			BeaconBlockRoot: make([]byte, fieldparams.RootLength),
			Target: &zondpb.Checkpoint{
				Epoch: currentEpoch,
				Root:  params.BeaconConfig().ZeroHash[:],
			},
			Source: &zondpb.Checkpoint{
				Epoch: currentEpoch,
				Root:  params.BeaconConfig().ZeroHash[:],
			},
		},
		AttestingIndices: []uint64{uint64(idx)},
	}
	att2.Signature, err = signing.ComputeDomainAndSign(bState, currentEpoch, att2.Data, params.BeaconConfig().DomainBeaconAttester, priv)
	if err != nil {
		return nil, err
	}

	return &zondpb.AttesterSlashing{
		Attestation_1: att1,
		Attestation_2: att2,
	}, nil
}

func generateAttesterSlashings(
	bState state.BeaconState,
	privs []bls.SecretKey,
	numSlashings uint64,
) ([]*zondpb.AttesterSlashing, error) {
	attesterSlashings := make([]*zondpb.AttesterSlashing, numSlashings)
	randGen := rand.NewDeterministicGenerator()
	for i := uint64(0); i < numSlashings; i++ {
		committeeIndex := randGen.Uint64() % helpers.SlotCommitteeCount(uint64(bState.NumValidators()))
		committee, err := helpers.BeaconCommitteeFromState(context.Background(), bState, bState.Slot(), primitives.CommitteeIndex(committeeIndex))
		if err != nil {
			return nil, err
		}
		randIndex := randGen.Uint64() % uint64(len(committee))
		valIndex := committee[randIndex]
		slashing, err := GenerateAttesterSlashingForValidator(bState, privs[valIndex], valIndex)
		if err != nil {
			return nil, err
		}
		attesterSlashings[i] = slashing
	}
	return attesterSlashings, nil
}

func generateDepositsAndEth1Data(
	bState state.BeaconState,
	numDeposits uint64,
) (
	[]*zondpb.Deposit,
	*zondpb.Eth1Data,
	error,
) {
	previousDepsLen := bState.Eth1DepositIndex()
	currentDeposits, _, err := DeterministicDepositsAndKeys(previousDepsLen + numDeposits)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get deposits")
	}
	eth1Data, err := DeterministicEth1Data(len(currentDeposits))
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get eth1data")
	}
	return currentDeposits[previousDepsLen:], eth1Data, nil
}

func GenerateVoluntaryExits(bState state.BeaconState, k bls.SecretKey, idx primitives.ValidatorIndex) (*zondpb.SignedVoluntaryExit, error) {
	currentEpoch := time.CurrentEpoch(bState)
	exit := &zondpb.SignedVoluntaryExit{
		Exit: &zondpb.VoluntaryExit{
			Epoch:          time.PrevEpoch(bState),
			ValidatorIndex: idx,
		},
	}
	var err error
	exit.Signature, err = signing.ComputeDomainAndSign(bState, currentEpoch, exit.Exit, params.BeaconConfig().DomainVoluntaryExit, k)
	if err != nil {
		return nil, err
	}
	return exit, nil
}

func generateVoluntaryExits(
	bState state.BeaconState,
	privs []bls.SecretKey,
	numExits uint64,
) ([]*zondpb.SignedVoluntaryExit, error) {
	currentEpoch := time.CurrentEpoch(bState)

	voluntaryExits := make([]*zondpb.SignedVoluntaryExit, numExits)
	valMap := map[primitives.ValidatorIndex]bool{}
	for i := 0; i < len(voluntaryExits); i++ {
		valIndex, err := randValIndex(bState)
		if err != nil {
			return nil, err
		}
		// Retry if validator exit already exists.
		if valMap[valIndex] {
			i--
			continue
		}
		exit := &zondpb.SignedVoluntaryExit{
			Exit: &zondpb.VoluntaryExit{
				Epoch:          time.PrevEpoch(bState),
				ValidatorIndex: valIndex,
			},
		}
		exit.Signature, err = signing.ComputeDomainAndSign(bState, currentEpoch, exit.Exit, params.BeaconConfig().DomainVoluntaryExit, privs[valIndex])
		if err != nil {
			return nil, err
		}
		voluntaryExits[i] = exit
		valMap[valIndex] = true
	}
	return voluntaryExits, nil
}

func randValIndex(bState state.BeaconState) (primitives.ValidatorIndex, error) {
	activeCount, err := helpers.ActiveValidatorCount(context.Background(), bState, time.CurrentEpoch(bState))
	if err != nil {
		return 0, err
	}
	return primitives.ValidatorIndex(rand.NewGenerator().Uint64() % activeCount), nil
}

// HydrateSignedBeaconHeader hydrates a signed beacon block header with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateSignedBeaconHeader(h *zondpb.SignedBeaconBlockHeader) *zondpb.SignedBeaconBlockHeader {
	if h.Signature == nil {
		h.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	h.Header = HydrateBeaconHeader(h.Header)
	return h
}

// HydrateBeaconHeader hydrates a beacon block header with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBeaconHeader(h *zondpb.BeaconBlockHeader) *zondpb.BeaconBlockHeader {
	if h == nil {
		h = &zondpb.BeaconBlockHeader{}
	}
	if h.BodyRoot == nil {
		h.BodyRoot = make([]byte, fieldparams.RootLength)
	}
	if h.StateRoot == nil {
		h.StateRoot = make([]byte, fieldparams.RootLength)
	}
	if h.ParentRoot == nil {
		h.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	return h
}

// HydrateSignedBeaconBlock hydrates a signed beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateSignedBeaconBlock(b *zondpb.SignedBeaconBlock) *zondpb.SignedBeaconBlock {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Block = HydrateBeaconBlock(b.Block)
	return b
}

// HydrateBeaconBlock hydrates a beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBeaconBlock(b *zondpb.BeaconBlock) *zondpb.BeaconBlock {
	if b == nil {
		b = &zondpb.BeaconBlock{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateBeaconBlockBody(b.Body)
	return b
}

// HydrateBeaconBlockBody hydrates a beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBeaconBlockBody(b *zondpb.BeaconBlockBody) *zondpb.BeaconBlockBody {
	if b == nil {
		b = &zondpb.BeaconBlockBody{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, fieldparams.RootLength)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &zondpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		}
	}
	return b
}

// HydrateV1SignedBeaconBlock hydrates a signed beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV1SignedBeaconBlock(b *v1.SignedBeaconBlock) *v1.SignedBeaconBlock {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Block = HydrateV1BeaconBlock(b.Block)
	return b
}

// HydrateV1BeaconBlock hydrates a beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV1BeaconBlock(b *v1.BeaconBlock) *v1.BeaconBlock {
	if b == nil {
		b = &v1.BeaconBlock{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateV1BeaconBlockBody(b.Body)
	return b
}

// HydrateV1BeaconBlockBody hydrates a beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV1BeaconBlockBody(b *v1.BeaconBlockBody) *v1.BeaconBlockBody {
	if b == nil {
		b = &v1.BeaconBlockBody{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, fieldparams.RootLength)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &v1.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		}
	}
	return b
}

// HydrateV2AltairSignedBeaconBlock hydrates a signed beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2AltairSignedBeaconBlock(b *v2.SignedBeaconBlockAltair) *v2.SignedBeaconBlockAltair {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Message = HydrateV2AltairBeaconBlock(b.Message)
	return b
}

// HydrateV2AltairBeaconBlock hydrates a beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2AltairBeaconBlock(b *v2.BeaconBlockAltair) *v2.BeaconBlockAltair {
	if b == nil {
		b = &v2.BeaconBlockAltair{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateV2AltairBeaconBlockBody(b.Body)
	return b
}

// HydrateV2AltairBeaconBlockBody hydrates a beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2AltairBeaconBlockBody(b *v2.BeaconBlockBodyAltair) *v2.BeaconBlockBodyAltair {
	if b == nil {
		b = &v2.BeaconBlockBodyAltair{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, fieldparams.RootLength)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &v1.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &v1.SyncAggregate{
			SyncCommitteeBits:      make([]byte, 64),
			SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
		}
	}
	return b
}

// HydrateV2BellatrixSignedBeaconBlock hydrates a signed beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2BellatrixSignedBeaconBlock(b *v2.SignedBeaconBlockBellatrix) *v2.SignedBeaconBlockBellatrix {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Message = HydrateV2BellatrixBeaconBlock(b.Message)
	return b
}

// HydrateV2BellatrixBeaconBlock hydrates a beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2BellatrixBeaconBlock(b *v2.BeaconBlockBellatrix) *v2.BeaconBlockBellatrix {
	if b == nil {
		b = &v2.BeaconBlockBellatrix{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateV2BellatrixBeaconBlockBody(b.Body)
	return b
}

// HydrateV2BellatrixBeaconBlockBody hydrates a beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2BellatrixBeaconBlockBody(b *v2.BeaconBlockBodyBellatrix) *v2.BeaconBlockBodyBellatrix {
	if b == nil {
		b = &v2.BeaconBlockBodyBellatrix{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, fieldparams.RootLength)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &v1.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &v1.SyncAggregate{
			SyncCommitteeBits:      make([]byte, 64),
			SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
		}
	}
	if b.ExecutionPayload == nil {
		b.ExecutionPayload = &enginev1.ExecutionPayload{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			ExtraData:     make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
		}
	}
	return b
}

// HydrateSignedBeaconBlockAltair hydrates a signed beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateSignedBeaconBlockAltair(b *zondpb.SignedBeaconBlockAltair) *zondpb.SignedBeaconBlockAltair {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Block = HydrateBeaconBlockAltair(b.Block)
	return b
}

// HydrateBeaconBlockAltair hydrates a beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBeaconBlockAltair(b *zondpb.BeaconBlockAltair) *zondpb.BeaconBlockAltair {
	if b == nil {
		b = &zondpb.BeaconBlockAltair{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateBeaconBlockBodyAltair(b.Body)
	return b
}

// HydrateBeaconBlockBodyAltair hydrates a beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBeaconBlockBodyAltair(b *zondpb.BeaconBlockBodyAltair) *zondpb.BeaconBlockBodyAltair {
	if b == nil {
		b = &zondpb.BeaconBlockBodyAltair{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, fieldparams.RootLength)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &zondpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &zondpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, 64),
			SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
		}
	}
	return b
}

// HydrateSignedBeaconBlockBellatrix hydrates a signed beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateSignedBeaconBlockBellatrix(b *zondpb.SignedBeaconBlockBellatrix) *zondpb.SignedBeaconBlockBellatrix {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Block = HydrateBeaconBlockBellatrix(b.Block)
	return b
}

// HydrateBeaconBlockBellatrix hydrates a beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBeaconBlockBellatrix(b *zondpb.BeaconBlockBellatrix) *zondpb.BeaconBlockBellatrix {
	if b == nil {
		b = &zondpb.BeaconBlockBellatrix{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateBeaconBlockBodyBellatrix(b.Body)
	return b
}

// HydrateBeaconBlockBodyBellatrix hydrates a beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBeaconBlockBodyBellatrix(b *zondpb.BeaconBlockBodyBellatrix) *zondpb.BeaconBlockBodyBellatrix {
	if b == nil {
		b = &zondpb.BeaconBlockBodyBellatrix{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, fieldparams.RootLength)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &zondpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &zondpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
		}
	}
	if b.ExecutionPayload == nil {
		b.ExecutionPayload = &enginev1.ExecutionPayload{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
			ExtraData:     make([]byte, 0),
		}
	}
	return b
}

// HydrateSignedBlindedBeaconBlockBellatrix hydrates a signed blinded beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateSignedBlindedBeaconBlockBellatrix(b *zondpb.SignedBlindedBeaconBlockBellatrix) *zondpb.SignedBlindedBeaconBlockBellatrix {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Block = HydrateBlindedBeaconBlockBellatrix(b.Block)
	return b
}

// HydrateBlindedBeaconBlockBellatrix hydrates a blinded beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBlindedBeaconBlockBellatrix(b *zondpb.BlindedBeaconBlockBellatrix) *zondpb.BlindedBeaconBlockBellatrix {
	if b == nil {
		b = &zondpb.BlindedBeaconBlockBellatrix{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateBlindedBeaconBlockBodyBellatrix(b.Body)
	return b
}

// HydrateBlindedBeaconBlockBodyBellatrix hydrates a blinded beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBlindedBeaconBlockBodyBellatrix(b *zondpb.BlindedBeaconBlockBodyBellatrix) *zondpb.BlindedBeaconBlockBodyBellatrix {
	if b == nil {
		b = &zondpb.BlindedBeaconBlockBodyBellatrix{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, 32)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &zondpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, 32),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &zondpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
		}
	}
	if b.ExecutionPayloadHeader == nil {
		b.ExecutionPayloadHeader = &enginev1.ExecutionPayloadHeader{
			ParentHash:       make([]byte, 32),
			FeeRecipient:     make([]byte, 20),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, 256),
			PrevRandao:       make([]byte, 32),
			BaseFeePerGas:    make([]byte, 32),
			BlockHash:        make([]byte, 32),
			TransactionsRoot: make([]byte, fieldparams.RootLength),
			ExtraData:        make([]byte, 0),
		}
	}
	return b
}

// HydrateV2SignedBlindedBeaconBlockBellatrix hydrates a signed blinded beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2SignedBlindedBeaconBlockBellatrix(b *v2.SignedBlindedBeaconBlockBellatrix) *v2.SignedBlindedBeaconBlockBellatrix {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Message = HydrateV2BlindedBeaconBlockBellatrix(b.Message)
	return b
}

// HydrateV2BlindedBeaconBlockBellatrix hydrates a blinded beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2BlindedBeaconBlockBellatrix(b *v2.BlindedBeaconBlockBellatrix) *v2.BlindedBeaconBlockBellatrix {
	if b == nil {
		b = &v2.BlindedBeaconBlockBellatrix{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateV2BlindedBeaconBlockBodyBellatrix(b.Body)
	return b
}

// HydrateV2BlindedBeaconBlockBodyBellatrix hydrates a blinded beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2BlindedBeaconBlockBodyBellatrix(b *v2.BlindedBeaconBlockBodyBellatrix) *v2.BlindedBeaconBlockBodyBellatrix {
	if b == nil {
		b = &v2.BlindedBeaconBlockBodyBellatrix{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, 32)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &v1.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, 32),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &v1.SyncAggregate{
			SyncCommitteeBits:      make([]byte, 64),
			SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
		}
	}
	if b.ExecutionPayloadHeader == nil {
		b.ExecutionPayloadHeader = &enginev1.ExecutionPayloadHeader{
			ParentHash:       make([]byte, 32),
			FeeRecipient:     make([]byte, 20),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, 256),
			PrevRandao:       make([]byte, 32),
			BaseFeePerGas:    make([]byte, 32),
			BlockHash:        make([]byte, 32),
			TransactionsRoot: make([]byte, fieldparams.RootLength),
		}
	}
	return b
}

// HydrateSignedBeaconBlockCapella hydrates a signed beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateSignedBeaconBlockCapella(b *zondpb.SignedBeaconBlockCapella) *zondpb.SignedBeaconBlockCapella {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Block = HydrateBeaconBlockCapella(b.Block)
	return b
}

// HydrateBeaconBlockCapella hydrates a beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBeaconBlockCapella(b *zondpb.BeaconBlockCapella) *zondpb.BeaconBlockCapella {
	if b == nil {
		b = &zondpb.BeaconBlockCapella{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateBeaconBlockBodyCapella(b.Body)
	return b
}

// HydrateBeaconBlockBodyCapella hydrates a beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBeaconBlockBodyCapella(b *zondpb.BeaconBlockBodyCapella) *zondpb.BeaconBlockBodyCapella {
	if b == nil {
		b = &zondpb.BeaconBlockBodyCapella{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, fieldparams.RootLength)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &zondpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &zondpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
		}
	}
	if b.ExecutionPayload == nil {
		b.ExecutionPayload = &enginev1.ExecutionPayloadCapella{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
			ExtraData:     make([]byte, 0),
		}
	}
	return b
}

// HydrateSignedBlindedBeaconBlockCapella hydrates a signed blinded beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateSignedBlindedBeaconBlockCapella(b *zondpb.SignedBlindedBeaconBlockCapella) *zondpb.SignedBlindedBeaconBlockCapella {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Block = HydrateBlindedBeaconBlockCapella(b.Block)
	return b
}

// HydrateBlindedBeaconBlockCapella hydrates a blinded beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBlindedBeaconBlockCapella(b *zondpb.BlindedBeaconBlockCapella) *zondpb.BlindedBeaconBlockCapella {
	if b == nil {
		b = &zondpb.BlindedBeaconBlockCapella{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateBlindedBeaconBlockBodyCapella(b.Body)
	return b
}

// HydrateBlindedBeaconBlockBodyCapella hydrates a blinded beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBlindedBeaconBlockBodyCapella(b *zondpb.BlindedBeaconBlockBodyCapella) *zondpb.BlindedBeaconBlockBodyCapella {
	if b == nil {
		b = &zondpb.BlindedBeaconBlockBodyCapella{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, 32)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &zondpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, 32),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &zondpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
		}
	}
	if b.ExecutionPayloadHeader == nil {
		b.ExecutionPayloadHeader = &enginev1.ExecutionPayloadHeaderCapella{
			ParentHash:       make([]byte, 32),
			FeeRecipient:     make([]byte, 20),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, 256),
			PrevRandao:       make([]byte, 32),
			BaseFeePerGas:    make([]byte, 32),
			BlockHash:        make([]byte, 32),
			TransactionsRoot: make([]byte, fieldparams.RootLength),
			ExtraData:        make([]byte, 0),
			WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
		}
	}
	return b
}

// HydrateV2SignedBlindedBeaconBlockCapella hydrates a signed blinded beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2SignedBlindedBeaconBlockCapella(b *v2.SignedBlindedBeaconBlockCapella) *v2.SignedBlindedBeaconBlockCapella {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Message = HydrateV2BlindedBeaconBlockCapella(b.Message)
	return b
}

// HydrateV2BlindedBeaconBlockCapella hydrates a blinded beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2BlindedBeaconBlockCapella(b *v2.BlindedBeaconBlockCapella) *v2.BlindedBeaconBlockCapella {
	if b == nil {
		b = &v2.BlindedBeaconBlockCapella{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateV2BlindedBeaconBlockBodyCapella(b.Body)
	return b
}

// HydrateV2BlindedBeaconBlockBodyCapella hydrates a blinded beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2BlindedBeaconBlockBodyCapella(b *v2.BlindedBeaconBlockBodyCapella) *v2.BlindedBeaconBlockBodyCapella {
	if b == nil {
		b = &v2.BlindedBeaconBlockBodyCapella{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, 32)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &v1.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, 32),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &v1.SyncAggregate{
			SyncCommitteeBits:      make([]byte, 64),
			SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
		}
	}
	if b.ExecutionPayloadHeader == nil {
		b.ExecutionPayloadHeader = &enginev1.ExecutionPayloadHeaderCapella{
			ParentHash:       make([]byte, 32),
			FeeRecipient:     make([]byte, 20),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, 256),
			PrevRandao:       make([]byte, 32),
			BaseFeePerGas:    make([]byte, 32),
			BlockHash:        make([]byte, 32),
			TransactionsRoot: make([]byte, fieldparams.RootLength),
			WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
		}
	}
	return b
}

func SaveBlock(tb assertions.AssertionTestingTB, ctx context.Context, db iface.NoHeadAccessDatabase, b interface{}) interfaces.SignedBeaconBlock {
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(tb, err)
	require.NoError(tb, db.SaveBlock(ctx, wsb))
	return wsb
}

// HydrateSignedBeaconBlockDeneb hydrates a signed beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateSignedBeaconBlockDeneb(b *zondpb.SignedBeaconBlockDeneb) *zondpb.SignedBeaconBlockDeneb {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Block = HydrateBeaconBlockDeneb(b.Block)
	return b
}

// HydrateV2SignedBeaconBlockDeneb hydrates a v2 signed beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2SignedBeaconBlockDeneb(b *v2.SignedBeaconBlockDeneb) *v2.SignedBeaconBlockDeneb {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Message = HydrateV2BeaconBlockDeneb(b.Message)
	return b
}

// HydrateBeaconBlockDeneb hydrates a beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBeaconBlockDeneb(b *zondpb.BeaconBlockDeneb) *zondpb.BeaconBlockDeneb {
	if b == nil {
		b = &zondpb.BeaconBlockDeneb{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateBeaconBlockBodyDeneb(b.Body)
	return b
}

// HydrateV2BeaconBlockDeneb hydrates a v2 beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2BeaconBlockDeneb(b *v2.BeaconBlockDeneb) *v2.BeaconBlockDeneb {
	if b == nil {
		b = &v2.BeaconBlockDeneb{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateV2BeaconBlockBodyDeneb(b.Body)
	return b
}

// HydrateBeaconBlockBodyDeneb hydrates a beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBeaconBlockBodyDeneb(b *zondpb.BeaconBlockBodyDeneb) *zondpb.BeaconBlockBodyDeneb {
	if b == nil {
		b = &zondpb.BeaconBlockBodyDeneb{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, fieldparams.RootLength)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &zondpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &zondpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
		}
	}
	if b.ExecutionPayload == nil {
		b.ExecutionPayload = &enginev1.ExecutionPayloadDeneb{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
			ExtraData:     make([]byte, 0),
		}
	}
	return b
}

// HydrateV2BeaconBlockBodyDeneb hydrates a v2 beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2BeaconBlockBodyDeneb(b *v2.BeaconBlockBodyDeneb) *v2.BeaconBlockBodyDeneb {
	if b == nil {
		b = &v2.BeaconBlockBodyDeneb{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, fieldparams.RootLength)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &v1.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &v1.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
		}
	}
	if b.ExecutionPayload == nil {
		b.ExecutionPayload = &enginev1.ExecutionPayloadDeneb{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
			ExtraData:     make([]byte, 0),
		}
	}
	return b
}

// HydrateSignedBlindedBeaconBlockDeneb hydrates a signed blinded beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateSignedBlindedBeaconBlockDeneb(b *zondpb.SignedBlindedBeaconBlockDeneb) *zondpb.SignedBlindedBeaconBlockDeneb {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Message = HydrateBlindedBeaconBlockDeneb(b.Message)
	return b
}

// HydrateV2SignedBlindedBeaconBlockDeneb hydrates a signed v2 blinded beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2SignedBlindedBeaconBlockDeneb(b *v2.SignedBlindedBeaconBlockDeneb) *v2.SignedBlindedBeaconBlockDeneb {
	if b.Signature == nil {
		b.Signature = make([]byte, dilithium2.CryptoBytes)
	}
	b.Message = HydrateV2BlindedBeaconBlockDeneb(b.Message)
	return b
}

// HydrateBlindedBeaconBlockDeneb hydrates a blinded beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBlindedBeaconBlockDeneb(b *zondpb.BlindedBeaconBlockDeneb) *zondpb.BlindedBeaconBlockDeneb {
	if b == nil {
		b = &zondpb.BlindedBeaconBlockDeneb{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateBlindedBeaconBlockBodyDeneb(b.Body)
	return b
}

// HydrateV2BlindedBeaconBlockDeneb hydrates a v2 blinded beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2BlindedBeaconBlockDeneb(b *v2.BlindedBeaconBlockDeneb) *v2.BlindedBeaconBlockDeneb {
	if b == nil {
		b = &v2.BlindedBeaconBlockDeneb{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateV2BlindedBeaconBlockBodyDeneb(b.Body)
	return b
}

// HydrateBlindedBeaconBlockBodyDeneb hydrates a blinded beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBlindedBeaconBlockBodyDeneb(b *zondpb.BlindedBeaconBlockBodyDeneb) *zondpb.BlindedBeaconBlockBodyDeneb {
	if b == nil {
		b = &zondpb.BlindedBeaconBlockBodyDeneb{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, 32)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &zondpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, 32),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &zondpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
		}
	}
	if b.ExecutionPayloadHeader == nil {
		b.ExecutionPayloadHeader = &enginev1.ExecutionPayloadHeaderDeneb{
			ParentHash:       make([]byte, 32),
			FeeRecipient:     make([]byte, 20),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, 256),
			PrevRandao:       make([]byte, 32),
			BaseFeePerGas:    make([]byte, 32),
			BlockHash:        make([]byte, 32),
			TransactionsRoot: make([]byte, fieldparams.RootLength),
			ExtraData:        make([]byte, 0),
			WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
		}
	}
	return b
}

// HydrateV2BlindedBeaconBlockBodyDeneb hydrates a blinded v2 beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV2BlindedBeaconBlockBodyDeneb(b *v2.BlindedBeaconBlockBodyDeneb) *v2.BlindedBeaconBlockBodyDeneb {
	if b == nil {
		b = &v2.BlindedBeaconBlockBodyDeneb{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, dilithium2.CryptoBytes)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, 32)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &v1.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, 32),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &v1.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
		}
	}
	if b.ExecutionPayloadHeader == nil {
		b.ExecutionPayloadHeader = &enginev1.ExecutionPayloadHeaderDeneb{
			ParentHash:       make([]byte, 32),
			FeeRecipient:     make([]byte, 20),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, 256),
			PrevRandao:       make([]byte, 32),
			BaseFeePerGas:    make([]byte, 32),
			BlockHash:        make([]byte, 32),
			TransactionsRoot: make([]byte, fieldparams.RootLength),
			ExtraData:        make([]byte, 0),
			WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
		}
	}
	return b
}
