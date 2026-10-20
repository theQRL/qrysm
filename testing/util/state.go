package util

import (
	"fmt"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// FillRootsNaturalOptZond is meant to be used as an option when calling NewBeaconStateZond.
// It fills state and block roots with hex representations of natural numbers starting with 0.
// Example: 16 becomes 0x00...0f.
func FillRootsNaturalOptZond(state *qrysmpb.BeaconStateZond) error {
	roots, err := PrepareRoots(int(params.BeaconConfig().SlotsPerHistoricalRoot))
	if err != nil {
		return err
	}
	state.StateRoots = roots
	state.BlockRoots = roots
	return nil
}

// NewBeaconStateZond creates a beacon state with minimum marshalable fields.
func NewBeaconStateZond(options ...func(state *qrysmpb.BeaconStateZond) error) (state.BeaconState, error) {
	pubkeys := make([][]byte, params.BeaconConfig().SyncCommitteeSize)
	for i := range pubkeys {
		pubkeys[i] = make([]byte, fieldparams.MLDSA87PubkeyLength)
	}

	seed := &qrysmpb.BeaconStateZond{
		BlockRoots:                 filledByteSlice2D(uint64(params.BeaconConfig().SlotsPerHistoricalRoot), 32),
		StateRoots:                 filledByteSlice2D(uint64(params.BeaconConfig().SlotsPerHistoricalRoot), 32),
		Slashings:                  make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector),
		RandaoMixes:                filledByteSlice2D(uint64(params.BeaconConfig().EpochsPerHistoricalVector), 32),
		Validators:                 make([]*qrysmpb.Validator, 0),
		CurrentJustifiedCheckpoint: &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		ExecutionData: &qrysmpb.ExecutionData{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, 32),
		},
		Fork: &qrysmpb.Fork{
			PreviousVersion: make([]byte, 4),
			CurrentVersion:  make([]byte, 4),
		},
		ExecutionDataVotes:          make([]*qrysmpb.ExecutionData, 0),
		HistoricalSummaries:         make([]*qrysmpb.HistoricalSummary, 0),
		JustificationBits:           bitfield.Bitvector4{0x0},
		FinalizedCheckpoint:         &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		LatestBlockHeader:           HydrateBeaconHeader(&qrysmpb.BeaconBlockHeader{}),
		PreviousJustifiedCheckpoint: &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		PreviousEpochParticipation:  make([]byte, 0),
		CurrentEpochParticipation:   make([]byte, 0),
		CurrentSyncCommittee: &qrysmpb.SyncCommittee{
			Pubkeys: pubkeys,
		},
		NextSyncCommittee: &qrysmpb.SyncCommittee{
			Pubkeys: pubkeys,
		},
		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderZond{
			ParentHash:       make([]byte, 32),
			FeeRecipient:     make([]byte, 64),
			StateRoot:        make([]byte, 32),
			ReceiptsRoot:     make([]byte, 32),
			LogsBloom:        make([]byte, 256),
			PrevRandao:       make([]byte, 32),
			ExtraData:        make([]byte, 0),
			BaseFeePerGas:    make([]byte, 32),
			BlockHash:        make([]byte, 32),
			TransactionsRoot: make([]byte, 32),
			WithdrawalsRoot:  make([]byte, 32),
		},
	}

	for _, opt := range options {
		err := opt(seed)
		if err != nil {
			return nil, err
		}
	}

	var st, err = state_native.InitializeFromProtoUnsafeZond(seed)
	if err != nil {
		return nil, err
	}

	return st.Copy(), nil
}

// SSZ will fill 2D byte slices with their respective values, so we must fill these in too for round
// trip testing.
func filledByteSlice2D(length, innerLen uint64) [][]byte {
	b := make([][]byte, length)
	for i := range length {
		b[i] = make([]byte, innerLen)
	}
	return b
}

// PrepareRoots returns a list of roots with hex representations of natural numbers starting with 0.
// Example: 16 becomes 0x00...0f.
func PrepareRoots(size int) ([][]byte, error) {
	roots := make([][]byte, size)
	for i := range size {
		roots[i] = make([]byte, fieldparams.RootLength)
	}
	for j := range roots {
		// Remove '0x' prefix and left-pad '0' to have 64 chars in total.
		s := fmt.Sprintf("%064s", hexutil.EncodeUint64(uint64(j))[2:])
		h, err := hexutil.Decode("0x" + s)
		if err != nil {
			return nil, err
		}
		roots[j] = h
	}
	return roots, nil
}
