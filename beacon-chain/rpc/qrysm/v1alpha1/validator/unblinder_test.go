package validator

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	builderTest "github.com/theQRL/qrysm/beacon-chain/builder/testing"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/encoding/ssz"
	v1 "github.com/theQRL/qrysm/proto/engine/v1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func Test_unblindBuilderBlock(t *testing.T) {
	p := emptyPayloadZond()
	p.GasLimit = 123

	tests := []struct {
		name        string
		blk         interfaces.SignedBeaconBlock
		mock        *builderTest.MockBuilderService
		err         string
		returnedBlk interfaces.SignedBeaconBlock
	}{
		{
			name: "old block version",
			blk: func() interfaces.SignedBeaconBlock {
				wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
				require.NoError(t, err)
				return wb
			}(),
			returnedBlk: func() interfaces.SignedBeaconBlock {
				wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
				require.NoError(t, err)
				return wb
			}(),
		},
		{
			name: "blinded without configured builder",
			blk: func() interfaces.SignedBeaconBlock {
				wb, err := blocks.NewSignedBeaconBlock(util.NewBlindedBeaconBlockZond())
				require.NoError(t, err)
				return wb
			}(),
			mock: &builderTest.MockBuilderService{
				HasConfigured: false,
			},
			err: "builder not configured",
		},
		{
			name: "non-blinded without configured builder",
			blk: func() interfaces.SignedBeaconBlock {
				b := util.NewBeaconBlockZond()
				b.Block.Slot = 1
				b.Block.ProposerIndex = 2
				b.Block.Body.ExecutionPayload = &v1.ExecutionPayloadZond{
					ParentHash:    make([]byte, fieldparams.RootLength),
					FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
					StateRoot:     make([]byte, fieldparams.RootLength),
					ReceiptsRoot:  make([]byte, fieldparams.RootLength),
					LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
					PrevRandao:    make([]byte, fieldparams.RootLength),
					BaseFeePerGas: make([]byte, fieldparams.RootLength),
					BlockHash:     make([]byte, fieldparams.RootLength),
					Transactions:  make([][]byte, 0),
					Withdrawals:   make([]*v1.Withdrawal, 0),
					GasLimit:      123,
				}
				wb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return wb
			}(),
			mock: &builderTest.MockBuilderService{
				HasConfigured: false,
				PayloadZond:   p,
			},
			returnedBlk: func() interfaces.SignedBeaconBlock {
				b := util.NewBeaconBlockZond()
				b.Block.Slot = 1
				b.Block.ProposerIndex = 2
				b.Block.Body.ExecutionPayload = p
				wb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return wb
			}(),
		},
		{
			name: "submit blind block error",
			blk: func() interfaces.SignedBeaconBlock {
				b := util.NewBlindedBeaconBlockZond()
				b.Block.Slot = 1
				b.Block.ProposerIndex = 2
				wb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return wb
			}(),
			mock: &builderTest.MockBuilderService{
				PayloadZond:           &v1.ExecutionPayloadZond{},
				HasConfigured:         true,
				ErrSubmitBlindedBlock: errors.New("can't submit"),
			},
			err: "can't submit",
		},
		{
			name: "head and payload root mismatch",
			blk: func() interfaces.SignedBeaconBlock {
				b := util.NewBlindedBeaconBlockZond()
				b.Block.Slot = 1
				b.Block.ProposerIndex = 2
				wb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return wb
			}(),
			mock: &builderTest.MockBuilderService{
				HasConfigured: true,
				PayloadZond:   p,
			},
			returnedBlk: func() interfaces.SignedBeaconBlock {
				b := util.NewBeaconBlockZond()
				b.Block.Slot = 1
				b.Block.ProposerIndex = 2
				b.Block.Body.ExecutionPayload = p
				wb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return wb
			}(),
			err: "header and payload root do not match",
		},
		{
			name: "can get payload Zond",
			blk: func() interfaces.SignedBeaconBlock {
				b := util.NewBlindedBeaconBlockZond()
				b.Block.Slot = 1
				b.Block.ProposerIndex = 2
				txRoot, err := ssz.TransactionsRoot([][]byte{})
				require.NoError(t, err)
				withdrawalsRoot, err := ssz.WithdrawalSliceRoot([]*v1.Withdrawal{}, fieldparams.MaxWithdrawalsPerPayload)
				require.NoError(t, err)
				b.Block.Body.ExecutionPayloadHeader = &v1.ExecutionPayloadHeaderZond{
					ParentHash:       make([]byte, fieldparams.RootLength),
					FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
					StateRoot:        make([]byte, fieldparams.RootLength),
					ReceiptsRoot:     make([]byte, fieldparams.RootLength),
					LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
					PrevRandao:       make([]byte, fieldparams.RootLength),
					BaseFeePerGas:    make([]byte, fieldparams.RootLength),
					BlockHash:        make([]byte, fieldparams.RootLength),
					TransactionsRoot: txRoot[:],
					WithdrawalsRoot:  withdrawalsRoot[:],
					GasLimit:         123,
				}
				wb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return wb
			}(),
			mock: &builderTest.MockBuilderService{
				HasConfigured: true,
				PayloadZond:   p,
			},
			returnedBlk: func() interfaces.SignedBeaconBlock {
				b := util.NewBeaconBlockZond()
				b.Block.Slot = 1
				b.Block.ProposerIndex = 2
				b.Block.Body.ExecutionPayload = p
				wb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return wb
			}(),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			unblinder, err := newUnblinder(tc.blk, tc.mock)
			require.NoError(t, err)
			gotBlk, err := unblinder.unblindBuilderBlock(context.Background())
			if tc.err != "" {
				require.ErrorContains(t, tc.err, err)
			} else {
				require.NoError(t, err)
				exec1, err := tc.returnedBlk.Block().Body().Execution()
				require.NoError(t, err)
				exec2, err := gotBlk.Block().Body().Execution()
				require.NoError(t, err)
				require.DeepEqual(t, exec1, exec2)
				require.DeepEqual(t, tc.returnedBlk, gotBlk)
			}
		})
	}
}
