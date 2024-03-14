package blocks_test

import (
	"encoding/hex"
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/time"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	consensusblocks "github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	"github.com/theQRL/qrysm/v4/encoding/ssz"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	"github.com/theQRL/qrysm/v4/time/slots"
)

func Test_IsExecutionBlock(t *testing.T) {
	tests := []struct {
		name    string
		payload *enginev1.ExecutionPayloadCapella
		want    bool
	}{
		{
			name:    "empty payload",
			payload: emptyPayloadCapella(),
			want:    false,
		},
		{
			name: "non-empty payload",
			payload: func() *enginev1.ExecutionPayloadCapella {
				p := emptyPayloadCapella()
				p.ParentHash = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return p
			}(),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blk := util.NewBeaconBlockCapella()
			blk.Block.Body.ExecutionPayload = tt.payload
			wrappedBlock, err := consensusblocks.NewBeaconBlock(blk.Block)
			require.NoError(t, err)
			got, err := blocks.IsExecutionBlock(wrappedBlock.Body())
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_IsExecutionBlockCapella(t *testing.T) {
	blk := util.NewBeaconBlockCapella()
	blk.Block.Body.ExecutionPayload = emptyPayloadCapella()
	wrappedBlock, err := consensusblocks.NewBeaconBlock(blk.Block)
	require.NoError(t, err)
	got, err := blocks.IsExecutionBlock(wrappedBlock.Body())
	require.NoError(t, err)
	require.Equal(t, false, got)
}

func Test_IsExecutionEnabled(t *testing.T) {
	tests := []struct {
		name        string
		payload     *enginev1.ExecutionPayloadCapella
		header      interfaces.ExecutionData
		useAltairSt bool
		want        bool
	}{
		{
			name:    "use older than bellatrix state",
			payload: emptyPayloadCapella(),
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				return h
			}(),
			useAltairSt: true,
			want:        false,
		},
		{
			name:    "empty header, empty payload",
			payload: emptyPayloadCapella(),
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				return h
			}(),
			want: false,
		},
		{
			name:    "non-empty header, empty payload",
			payload: emptyPayloadCapella(),
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				p, ok := h.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				require.Equal(t, true, ok)
				p.ParentHash = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return h
			}(),
			want: true,
		},
		{
			name: "empty header, non-empty payload",
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				return h
			}(),
			payload: func() *enginev1.ExecutionPayloadCapella {
				p := emptyPayloadCapella()
				p.Timestamp = 1
				return p
			}(),
			want: true,
		},
		{
			name: "non-empty header, non-empty payload",
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				p, ok := h.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				require.Equal(t, true, ok)
				p.ParentHash = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return h
			}(),
			payload: func() *enginev1.ExecutionPayloadCapella {
				p := emptyPayloadCapella()
				p.Timestamp = 1
				return p
			}(),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, _ := util.DeterministicGenesisStateCapella(t, 1)
			require.NoError(t, st.SetLatestExecutionPayloadHeader(tt.header))
			blk := util.NewBeaconBlockCapella()
			blk.Block.Body.ExecutionPayload = tt.payload
			body, err := consensusblocks.NewBeaconBlockBody(blk.Block.Body)
			require.NoError(t, err)
			if tt.useAltairSt {
				st, _ = util.DeterministicGenesisStateCapella(t, 1)
			}
			got, err := blocks.IsExecutionEnabled(st, body)
			require.NoError(t, err)
			if got != tt.want {
				t.Errorf("IsExecutionEnabled() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_IsExecutionEnabledUsingHeader(t *testing.T) {
	tests := []struct {
		name    string
		payload *enginev1.ExecutionPayloadCapella
		header  interfaces.ExecutionData
		want    bool
	}{
		{
			name:    "empty header, empty payload",
			payload: emptyPayloadCapella(),
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				return h
			}(),
			want: false,
		},
		{
			name:    "non-empty header, empty payload",
			payload: emptyPayloadCapella(),
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				p, ok := h.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				require.Equal(t, true, ok)
				p.ParentHash = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return h
			}(),
			want: true,
		},
		{
			name: "empty header, non-empty payload",
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				return h
			}(),
			payload: func() *enginev1.ExecutionPayloadCapella {
				p := emptyPayloadCapella()
				p.Timestamp = 1
				return p
			}(),
			want: true,
		},
		{
			name: "non-empty header, non-empty payload",
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				p, ok := h.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				require.Equal(t, true, ok)
				p.ParentHash = bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength)
				return h
			}(),
			payload: func() *enginev1.ExecutionPayloadCapella {
				p := emptyPayloadCapella()
				p.Timestamp = 1
				return p
			}(),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blk := util.NewBeaconBlockCapella()
			blk.Block.Body.ExecutionPayload = tt.payload
			body, err := consensusblocks.NewBeaconBlockBody(blk.Block.Body)
			require.NoError(t, err)
			got, err := blocks.IsExecutionEnabledUsingHeader(tt.header, body)
			require.NoError(t, err)
			if got != tt.want {
				t.Errorf("IsExecutionEnabled() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ValidatePayload(t *testing.T) {
	st, _ := util.DeterministicGenesisStateCapella(t, 1)
	random, err := helpers.RandaoMix(st, time.CurrentEpoch(st))
	require.NoError(t, err)
	ts, err := slots.ToTime(st.GenesisTime(), st.Slot())
	require.NoError(t, err)
	tests := []struct {
		name    string
		payload *enginev1.ExecutionPayloadCapella
		err     error
	}{
		{
			name: "validate passes",
			payload: func() *enginev1.ExecutionPayloadCapella {
				h := emptyPayloadCapella()
				h.PrevRandao = random
				h.Timestamp = uint64(ts.Unix())
				return h
			}(), err: nil,
		},
		{
			name:    "incorrect prev randao",
			payload: emptyPayloadCapella(),
			err:     blocks.ErrInvalidPayloadPrevRandao,
		},
		{
			name: "incorrect timestamp",
			payload: func() *enginev1.ExecutionPayloadCapella {
				h := emptyPayloadCapella()
				h.PrevRandao = random
				h.Timestamp = 1
				return h
			}(),
			err: blocks.ErrInvalidPayloadTimeStamp,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrappedPayload, err := consensusblocks.WrappedExecutionPayloadCapella(tt.payload, 0)
			require.NoError(t, err)
			err = blocks.ValidatePayload(st, wrappedPayload)
			if err != nil {
				require.Equal(t, tt.err.Error(), err.Error())
			} else {
				require.Equal(t, tt.err, err)
			}
		})
	}
}

func Test_ProcessPayload(t *testing.T) {
	st, _ := util.DeterministicGenesisStateCapella(t, 1)
	random, err := helpers.RandaoMix(st, time.CurrentEpoch(st))
	require.NoError(t, err)
	ts, err := slots.ToTime(st.GenesisTime(), st.Slot())
	require.NoError(t, err)
	tests := []struct {
		name    string
		payload *enginev1.ExecutionPayloadCapella
		err     error
	}{
		{
			name: "process passes",
			payload: func() *enginev1.ExecutionPayloadCapella {
				h := emptyPayloadCapella()
				h.PrevRandao = random
				h.Timestamp = uint64(ts.Unix())
				return h
			}(), err: nil,
		},
		{
			name:    "incorrect prev randao",
			payload: emptyPayloadCapella(),
			err:     blocks.ErrInvalidPayloadPrevRandao,
		},
		{
			name: "incorrect timestamp",
			payload: func() *enginev1.ExecutionPayloadCapella {
				h := emptyPayloadCapella()
				h.PrevRandao = random
				h.Timestamp = 1
				return h
			}(),
			err: blocks.ErrInvalidPayloadTimeStamp,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrappedPayload, err := consensusblocks.WrappedExecutionPayloadCapella(tt.payload, 0)
			require.NoError(t, err)
			st, err := blocks.ProcessPayload(st, wrappedPayload)
			if err != nil {
				require.Equal(t, tt.err.Error(), err.Error())
			} else {
				require.Equal(t, tt.err, err)
				want, err := consensusblocks.PayloadToHeaderCapella(wrappedPayload)
				require.Equal(t, tt.err, err)
				h, err := st.LatestExecutionPayloadHeader()
				require.NoError(t, err)
				got, ok := h.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				require.Equal(t, true, ok)
				require.DeepSSZEqual(t, want, got)
			}
		})
	}
}

func Test_ProcessPayloadCapella(t *testing.T) {
	st, _ := util.DeterministicGenesisStateCapella(t, 1)
	header, err := emptyPayloadHeaderCapella()
	require.NoError(t, err)
	require.NoError(t, st.SetLatestExecutionPayloadHeader(header))
	payload := emptyPayloadCapella()
	random, err := helpers.RandaoMix(st, time.CurrentEpoch(st))
	require.NoError(t, err)
	payload.PrevRandao = random
	wrapped, err := consensusblocks.WrappedExecutionPayloadCapella(payload, 0)
	require.NoError(t, err)
	_, err = blocks.ProcessPayload(st, wrapped)
	require.NoError(t, err)
}

func Test_ProcessPayloadHeader(t *testing.T) {
	st, _ := util.DeterministicGenesisStateCapella(t, 1)
	random, err := helpers.RandaoMix(st, time.CurrentEpoch(st))
	require.NoError(t, err)
	ts, err := slots.ToTime(st.GenesisTime(), st.Slot())
	require.NoError(t, err)
	wrappedHeader, err := consensusblocks.WrappedExecutionPayloadHeaderCapella(&enginev1.ExecutionPayloadHeaderCapella{BlockHash: []byte{'a'}}, 0)
	require.NoError(t, err)
	require.NoError(t, st.SetLatestExecutionPayloadHeader(wrappedHeader))
	wdRoot, err := hex.DecodeString("792930bbd5baac43bcc798ee49aa8185ef76bb3b44ba62b91d86ae569e4bb535")
	require.NoError(t, err)
	tests := []struct {
		name   string
		header interfaces.ExecutionData
		err    error
	}{
		{
			name: "process passes",
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				p, ok := h.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				require.Equal(t, true, ok)
				p.ParentHash = []byte{'a'}
				p.PrevRandao = random
				p.Timestamp = uint64(ts.Unix())
				p.WithdrawalsRoot = wdRoot
				return h
			}(), err: nil,
		},
		{
			name: "incorrect prev randao",
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				p, ok := h.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				require.Equal(t, true, ok)
				p.WithdrawalsRoot = wdRoot
				return h
			}(),
			err: blocks.ErrInvalidPayloadPrevRandao,
		},
		{
			name: "incorrect timestamp",
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				p, ok := h.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				require.Equal(t, true, ok)
				p.PrevRandao = random
				p.Timestamp = 1
				p.WithdrawalsRoot = wdRoot
				return h
			}(),
			err: blocks.ErrInvalidPayloadTimeStamp,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, err := blocks.ProcessPayloadHeader(st, tt.header)
			if err != nil {
				require.Equal(t, tt.err.Error(), err.Error())
			} else {
				require.Equal(t, tt.err, err)
				want, ok := tt.header.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				require.Equal(t, true, ok)
				h, err := st.LatestExecutionPayloadHeader()
				require.NoError(t, err)
				got, ok := h.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				require.Equal(t, true, ok)
				require.DeepSSZEqual(t, want, got)
			}
		})
	}
}

func Test_ValidatePayloadHeader(t *testing.T) {
	st, _ := util.DeterministicGenesisStateCapella(t, 1)
	random, err := helpers.RandaoMix(st, time.CurrentEpoch(st))
	require.NoError(t, err)
	ts, err := slots.ToTime(st.GenesisTime(), st.Slot())
	require.NoError(t, err)
	tests := []struct {
		name   string
		header interfaces.ExecutionData
		err    error
	}{
		{
			name: "process passes",
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				p, ok := h.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				require.Equal(t, true, ok)
				p.PrevRandao = random
				p.Timestamp = uint64(ts.Unix())
				return h
			}(), err: nil,
		},
		{
			name: "incorrect prev randao",
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				return h
			}(),
			err: blocks.ErrInvalidPayloadPrevRandao,
		},
		{
			name: "incorrect timestamp",
			header: func() interfaces.ExecutionData {
				h, err := emptyPayloadHeaderCapella()
				require.NoError(t, err)
				p, ok := h.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				require.Equal(t, true, ok)
				p.PrevRandao = random
				p.Timestamp = 1
				return h
			}(),
			err: blocks.ErrInvalidPayloadTimeStamp,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err = blocks.ValidatePayloadHeader(st, tt.header)
			require.Equal(t, tt.err, err)
		})
	}
}

func Test_PayloadToHeader(t *testing.T) {
	p := emptyPayloadCapella()
	wrappedPayload, err := consensusblocks.WrappedExecutionPayloadCapella(p, 0)
	require.NoError(t, err)
	h, err := consensusblocks.PayloadToHeaderCapella(wrappedPayload)
	require.NoError(t, err)
	txRoot, err := ssz.TransactionsRoot(p.Transactions)
	require.NoError(t, err)
	require.DeepSSZEqual(t, txRoot, bytesutil.ToBytes32(h.TransactionsRoot))

	// Verify copy works
	b := []byte{'a'}
	p.ParentHash = b
	p.FeeRecipient = b
	p.StateRoot = b
	p.ReceiptsRoot = b
	p.LogsBloom = b
	p.PrevRandao = b
	p.ExtraData = b
	p.BaseFeePerGas = b
	p.BlockHash = b
	p.BlockNumber = 1
	p.GasUsed = 1
	p.GasLimit = 1
	p.Timestamp = 1

	require.DeepSSZEqual(t, h.ParentHash, make([]byte, fieldparams.RootLength))
	require.DeepSSZEqual(t, h.FeeRecipient, make([]byte, fieldparams.FeeRecipientLength))
	require.DeepSSZEqual(t, h.StateRoot, make([]byte, fieldparams.RootLength))
	require.DeepSSZEqual(t, h.ReceiptsRoot, make([]byte, fieldparams.RootLength))
	require.DeepSSZEqual(t, h.LogsBloom, make([]byte, fieldparams.LogsBloomLength))
	require.DeepSSZEqual(t, h.PrevRandao, make([]byte, fieldparams.RootLength))
	require.DeepSSZEqual(t, h.ExtraData, make([]byte, 0))
	require.DeepSSZEqual(t, h.BaseFeePerGas, make([]byte, fieldparams.RootLength))
	require.DeepSSZEqual(t, h.BlockHash, make([]byte, fieldparams.RootLength))
	require.Equal(t, h.BlockNumber, uint64(0))
	require.Equal(t, h.GasUsed, uint64(0))
	require.Equal(t, h.GasLimit, uint64(0))
	require.Equal(t, h.Timestamp, uint64(0))
}

func emptyPayloadHeaderCapella() (interfaces.ExecutionData, error) {
	return consensusblocks.WrappedExecutionPayloadHeaderCapella(&enginev1.ExecutionPayloadHeaderCapella{
		ParentHash:       make([]byte, fieldparams.RootLength),
		FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
		StateRoot:        make([]byte, fieldparams.RootLength),
		ReceiptsRoot:     make([]byte, fieldparams.RootLength),
		LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
		PrevRandao:       make([]byte, fieldparams.RootLength),
		BaseFeePerGas:    make([]byte, fieldparams.RootLength),
		BlockHash:        make([]byte, fieldparams.RootLength),
		TransactionsRoot: make([]byte, fieldparams.RootLength),
		WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
		ExtraData:        make([]byte, 0),
	}, 0)
}

func emptyPayloadCapella() *enginev1.ExecutionPayloadCapella {
	return &enginev1.ExecutionPayloadCapella{
		ParentHash:    make([]byte, fieldparams.RootLength),
		FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
		StateRoot:     make([]byte, fieldparams.RootLength),
		ReceiptsRoot:  make([]byte, fieldparams.RootLength),
		LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
		PrevRandao:    make([]byte, fieldparams.RootLength),
		BaseFeePerGas: make([]byte, fieldparams.RootLength),
		BlockHash:     make([]byte, fieldparams.RootLength),
		Transactions:  make([][]byte, 0),
		Withdrawals:   make([]*enginev1.Withdrawal, 0),
		ExtraData:     make([]byte, 0),
	}
}
