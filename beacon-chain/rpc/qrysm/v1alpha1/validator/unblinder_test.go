package validator

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	builderTest "github.com/theQRL/qrysm/v4/beacon-chain/builder/testing"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/encoding/ssz"
	v1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zond "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func Test_unblindBuilderBlock(t *testing.T) {
	p := emptyPayloadCapella()
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
				wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
				require.NoError(t, err)
				return wb
			}(),
			returnedBlk: func() interfaces.SignedBeaconBlock {
				wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
				require.NoError(t, err)
				return wb
			}(),
		},
		{
			name: "blinded without configured builder",
			blk: func() interfaces.SignedBeaconBlock {
				wb, err := blocks.NewSignedBeaconBlock(util.NewBlindedBeaconBlockCapella())
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
				b := util.NewBeaconBlockCapella()
				b.Block.Slot = 1
				b.Block.ProposerIndex = 2
				b.Block.Body.ExecutionPayload = &v1.ExecutionPayloadCapella{
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
				HasConfigured:  false,
				PayloadCapella: p,
			},
			returnedBlk: func() interfaces.SignedBeaconBlock {
				b := util.NewBeaconBlockCapella()
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
				b := util.NewBlindedBeaconBlockCapella()
				b.Block.Slot = 1
				b.Block.ProposerIndex = 2
				wb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return wb
			}(),
			mock: &builderTest.MockBuilderService{
				PayloadCapella:        &v1.ExecutionPayloadCapella{},
				HasConfigured:         true,
				ErrSubmitBlindedBlock: errors.New("can't submit"),
			},
			err: "can't submit",
		},
		{
			name: "head and payload root mismatch",
			blk: func() interfaces.SignedBeaconBlock {
				b := util.NewBlindedBeaconBlockCapella()
				b.Block.Slot = 1
				b.Block.ProposerIndex = 2
				wb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return wb
			}(),
			mock: &builderTest.MockBuilderService{
				HasConfigured:  true,
				PayloadCapella: p,
			},
			returnedBlk: func() interfaces.SignedBeaconBlock {
				b := util.NewBeaconBlockCapella()
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
			name: "can get payload Capella",
			blk: func() interfaces.SignedBeaconBlock {
				b := util.NewBlindedBeaconBlockCapella()
				b.Block.Slot = 1
				b.Block.ProposerIndex = 2
				b.Block.Body.DilithiumToExecutionChanges = []*zond.SignedDilithiumToExecutionChange{
					{
						Message: &zond.DilithiumToExecutionChange{
							ValidatorIndex:      123,
							FromDilithiumPubkey: []byte{'a'},
							ToExecutionAddress:  []byte{'a'},
						},
						Signature: []byte("sig123"),
					},
					{
						Message: &zond.DilithiumToExecutionChange{
							ValidatorIndex:      456,
							FromDilithiumPubkey: []byte{'b'},
							ToExecutionAddress:  []byte{'b'},
						},
						Signature: []byte("sig456"),
					},
				}
				txRoot, err := ssz.TransactionsRoot([][]byte{})
				require.NoError(t, err)
				withdrawalsRoot, err := ssz.WithdrawalSliceRoot([]*v1.Withdrawal{}, fieldparams.MaxWithdrawalsPerPayload)
				require.NoError(t, err)
				b.Block.Body.ExecutionPayloadHeader = &v1.ExecutionPayloadHeaderCapella{
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
				HasConfigured:  true,
				PayloadCapella: p,
			},
			returnedBlk: func() interfaces.SignedBeaconBlock {
				b := util.NewBeaconBlockCapella()
				b.Block.Slot = 1
				b.Block.ProposerIndex = 2
				b.Block.Body.DilithiumToExecutionChanges = []*zond.SignedDilithiumToExecutionChange{
					{
						Message: &zond.DilithiumToExecutionChange{
							ValidatorIndex:      123,
							FromDilithiumPubkey: []byte{'a'},
							ToExecutionAddress:  []byte{'a'},
						},
						Signature: []byte("sig123"),
					},
					{
						Message: &zond.DilithiumToExecutionChange{
							ValidatorIndex:      456,
							FromDilithiumPubkey: []byte{'b'},
							ToExecutionAddress:  []byte{'b'},
						},
						Signature: []byte("sig456"),
					},
				}
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
