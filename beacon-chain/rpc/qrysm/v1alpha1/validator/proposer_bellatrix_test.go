package validator

import (
	"context"
	"math/big"
	"testing"
	"time"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/api/client/builder"
	blockchainTest "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	builderTest "github.com/theQRL/qrysm/beacon-chain/builder/testing"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	dbTest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	exectesting "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	consensus_types "github.com/theQRL/qrysm/consensus-types"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/encoding/ssz"
	v1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
)

func TestServer_setExecutionData(t *testing.T) {
	hook := logTest.NewGlobal()

	ctx := context.Background()
	params.SetupTestConfigCleanup(t)

	gasLimit := uint64(20000000) // matches go-qrl params.MaxGasLimit
	beaconDB := dbTest.SetupDB(t)
	zondTransitionState, _ := util.DeterministicGenesisStateZond(t, 1)
	wrappedHeaderZond, err := blocks.WrappedExecutionPayloadHeaderZond(&v1.ExecutionPayloadHeaderZond{BlockNumber: 1, GasLimit: gasLimit}, 0)
	require.NoError(t, err)
	require.NoError(t, zondTransitionState.SetLatestExecutionPayloadHeader(wrappedHeaderZond))
	b2pbZond := util.NewBeaconBlockZond()
	b2rZond, err := b2pbZond.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b2pbZond)
	require.NoError(t, zondTransitionState.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{
		Root: b2rZond[:],
	}))
	require.NoError(t, beaconDB.SaveFeeRecipientsByValidatorIDs(context.Background(), []primitives.ValidatorIndex{0}, []common.Address{{}}))

	withdrawals := []*v1.Withdrawal{{
		Index:          1,
		ValidatorIndex: 2,
		Address:        make([]byte, fieldparams.FeeRecipientLength),
		Amount:         3,
	}}
	id := &v1.PayloadIDBytes{0x1}
	vs := &Server{
		ExecutionEngineCaller:  &exectesting.EngineClient{PayloadIDBytes: id, ExecutionPayloadZond: &v1.ExecutionPayloadZond{BlockNumber: 1, Withdrawals: withdrawals}, BlockValue: 0},
		HeadFetcher:            &blockchainTest.ChainService{State: zondTransitionState},
		FinalizationFetcher:    &blockchainTest.ChainService{},
		BeaconDB:               beaconDB,
		ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
		BlockBuilder:           &builderTest.MockBuilderService{HasConfigured: true, Cfg: &builderTest.Config{BeaconDB: beaconDB}},
		ForkchoiceFetcher:      &blockchainTest.ChainService{},
	}

	t.Run("No builder configured. Use local block", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
		require.NoError(t, err)
		b := blk.Block()
		localPayload, _, err := vs.getLocalPayload(ctx, b, zondTransitionState)
		require.NoError(t, err)
		builderPayload, err := vs.getBuilderPayload(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.NoError(t, err)
		require.NoError(t, setExecutionData(context.Background(), blk, localPayload, builderPayload))
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(1), e.BlockNumber()) // Local block
	})
	t.Run("Builder configured. Builder Block has higher value. Incorrect withdrawals", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
		require.NoError(t, err)
		require.NoError(t, vs.BeaconDB.SaveRegistrationsByValidatorIDs(ctx, []primitives.ValidatorIndex{blk.Block().ProposerIndex()},
			[]*qrysmpb.ValidatorRegistrationV1{{FeeRecipient: make([]byte, fieldparams.FeeRecipientLength), Timestamp: uint64(time.Now().Unix()), GasLimit: gasLimit, Pubkey: make([]byte, field_params.MLDSA87PubkeyLength)}}))
		ti, err := slots.ToTime(uint64(time.Now().Unix()), 0)
		require.NoError(t, err)
		sk, err := ml_dsa_87.RandKey()
		require.NoError(t, err)
		bid := &qrysmpb.BuilderBidZond{
			Header: &v1.ExecutionPayloadHeaderZond{
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
				ParentHash:       params.BeaconConfig().ZeroHash[:],
				Timestamp:        uint64(ti.Unix()),
				BlockNumber:      2,
				WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
				GasLimit:         gasLimit,
			},
			Pubkey: sk.PublicKey().Marshal(),
			Value:  bytesutil.PadTo([]byte{1}, 32),
		}
		d := params.BeaconConfig().DomainApplicationBuilder
		domain, err := signing.ComputeDomain(d, nil, nil)
		require.NoError(t, err)
		sr, err := signing.ComputeSigningRoot(bid, domain)
		require.NoError(t, err)
		sBid := &qrysmpb.SignedBuilderBidZond{
			Message:   bid,
			Signature: sk.Sign(sr[:]).Marshal(),
		}
		vs.BlockBuilder = &builderTest.MockBuilderService{
			BidZond:       sBid,
			HasConfigured: true,
			Cfg:           &builderTest.Config{BeaconDB: beaconDB},
		}
		wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
		require.NoError(t, err)
		chain := &blockchainTest.ChainService{ForkChoiceStore: doublylinkedtree.New(), Genesis: time.Now(), Block: wb}
		vs.ForkchoiceFetcher = chain
		vs.ForkchoiceFetcher.SetForkChoiceGenesisTime(uint64(time.Now().Unix()))
		vs.TimeFetcher = chain
		vs.HeadFetcher = chain
		b := blk.Block()

		localPayload, _, err := vs.getLocalPayload(ctx, b, zondTransitionState)
		require.NoError(t, err)
		builderPayload, err := vs.getBuilderPayload(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.NoError(t, err)
		require.NoError(t, setExecutionData(context.Background(), blk, localPayload, builderPayload))
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(1), e.BlockNumber()) // Local block because incorrect withdrawals
	})
	t.Run("Builder configured. Builder Block has higher value. Correct withdrawals.", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBlindedBeaconBlockZond())
		require.NoError(t, err)
		require.NoError(t, vs.BeaconDB.SaveRegistrationsByValidatorIDs(ctx, []primitives.ValidatorIndex{blk.Block().ProposerIndex()},
			[]*qrysmpb.ValidatorRegistrationV1{{FeeRecipient: make([]byte, fieldparams.FeeRecipientLength), Timestamp: uint64(time.Now().Unix()), GasLimit: gasLimit, Pubkey: make([]byte, field_params.MLDSA87PubkeyLength)}}))
		ti, err := slots.ToTime(uint64(time.Now().Unix()), 0)
		require.NoError(t, err)
		sk, err := ml_dsa_87.RandKey()
		require.NoError(t, err)
		wr, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
		require.NoError(t, err)
		builderValue := bytesutil.ReverseByteOrder(big.NewInt(1e9).Bytes())
		bid := &qrysmpb.BuilderBidZond{
			Header: &v1.ExecutionPayloadHeaderZond{
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
				ParentHash:       params.BeaconConfig().ZeroHash[:],
				Timestamp:        uint64(ti.Unix()),
				BlockNumber:      2,
				WithdrawalsRoot:  wr[:],
				GasLimit:         gasLimit,
			},
			Pubkey: sk.PublicKey().Marshal(),
			Value:  bytesutil.PadTo(builderValue, 32),
		}
		d := params.BeaconConfig().DomainApplicationBuilder
		domain, err := signing.ComputeDomain(d, nil, nil)
		require.NoError(t, err)
		sr, err := signing.ComputeSigningRoot(bid, domain)
		require.NoError(t, err)
		sBid := &qrysmpb.SignedBuilderBidZond{
			Message:   bid,
			Signature: sk.Sign(sr[:]).Marshal(),
		}
		vs.BlockBuilder = &builderTest.MockBuilderService{
			BidZond:       sBid,
			HasConfigured: true,
			Cfg:           &builderTest.Config{BeaconDB: beaconDB},
		}
		wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
		require.NoError(t, err)
		chain := &blockchainTest.ChainService{ForkChoiceStore: doublylinkedtree.New(), Genesis: time.Now(), Block: wb}
		vs.ForkFetcher = chain
		vs.ForkchoiceFetcher.SetForkChoiceGenesisTime(uint64(time.Now().Unix()))
		vs.TimeFetcher = chain
		vs.HeadFetcher = chain

		b := blk.Block()
		localPayload, _, err := vs.getLocalPayload(ctx, b, zondTransitionState)
		require.NoError(t, err)
		builderPayload, err := vs.getBuilderPayload(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.NoError(t, err)
		require.NoError(t, setExecutionData(context.Background(), blk, localPayload, builderPayload))
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(2), e.BlockNumber()) // Builder block
	})
	t.Run("Builder configured. Local block has higher value", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
		require.NoError(t, err)
		vs.ExecutionEngineCaller = &exectesting.EngineClient{PayloadIDBytes: id, ExecutionPayloadZond: &v1.ExecutionPayloadZond{BlockNumber: 3}, BlockValue: 2}
		b := blk.Block()
		localPayload, _, err := vs.getLocalPayload(ctx, b, zondTransitionState)
		require.NoError(t, err)
		require.NoError(t, err)
		builderPayload, err := vs.getBuilderPayload(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.NoError(t, err)
		require.NoError(t, setExecutionData(context.Background(), blk, localPayload, builderPayload))
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(3), e.BlockNumber()) // Local block

		require.LogsContain(t, hook, "builderShorValue=1 localBoostPercentage=0 localShorValue=2")
	})
	t.Run("Builder configured. Local block and boost has higher value", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.LocalBlockValueBoost = 1 // Boost 1%.
		params.OverrideBeaconConfig(cfg)

		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
		require.NoError(t, err)
		vs.ExecutionEngineCaller = &exectesting.EngineClient{PayloadIDBytes: id, ExecutionPayloadZond: &v1.ExecutionPayloadZond{BlockNumber: 3}, BlockValue: 1}
		b := blk.Block()
		localPayload, _, err := vs.getLocalPayload(ctx, b, zondTransitionState)
		require.NoError(t, err)
		builderPayload, err := vs.getBuilderPayload(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.NoError(t, err)
		require.NoError(t, setExecutionData(context.Background(), blk, localPayload, builderPayload))
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(3), e.BlockNumber()) // Local block

		require.LogsContain(t, hook, "builderShorValue=1 localBoostPercentage=1 localShorValue=1")
	})
	t.Run("Builder configured. Builder returns fault. Use local block", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
		require.NoError(t, err)
		vs.BlockBuilder = &builderTest.MockBuilderService{
			// ErrGetHeader:  errors.New("fault"),
			HasConfigured: true,
			Cfg:           &builderTest.Config{BeaconDB: beaconDB},
		}
		vs.ExecutionEngineCaller = &exectesting.EngineClient{PayloadIDBytes: id, ExecutionPayloadZond: &v1.ExecutionPayloadZond{BlockNumber: 4}, BlockValue: 0}
		b := blk.Block()
		localPayload, _, err := vs.getLocalPayload(ctx, b, zondTransitionState)
		require.NoError(t, err)
		builderPayload, err := vs.getBuilderPayload(ctx, b.Slot(), b.ProposerIndex(), gasLimit)
		require.ErrorIs(t, consensus_types.ErrNilObjectWrapped, err) // Builder returns fault. Use local block
		require.NoError(t, setExecutionData(context.Background(), blk, localPayload, builderPayload))
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(4), e.BlockNumber()) // Local block
	})
}

func TestServer_getPayloadHeader(t *testing.T) {
	genesis := time.Now().Add(-time.Duration(params.BeaconConfig().SlotsPerEpoch) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	params.SetupTestConfigCleanup(t)
	fakeZondEpoch := primitives.Epoch(0)
	emptyRoot, err := ssz.TransactionsRoot([][]byte{})
	require.NoError(t, err)

	sk, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	d := params.BeaconConfig().DomainApplicationBuilder
	domain, err := signing.ComputeDomain(d, nil, nil)
	require.NoError(t, err)
	withdrawals := []*v1.Withdrawal{{
		Index:          1,
		ValidatorIndex: 2,
		Address:        make([]byte, fieldparams.FeeRecipientLength),
		Amount:         3,
	}}
	wr, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
	require.NoError(t, err)

	tiZond, err := slots.ToTime(uint64(genesis.Unix()), primitives.Slot(fakeZondEpoch)*params.BeaconConfig().SlotsPerEpoch)
	require.NoError(t, err)
	bidZond := &qrysmpb.BuilderBidZond{
		Header: &v1.ExecutionPayloadHeaderZond{
			FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:       make([]byte, fieldparams.RootLength),
			BaseFeePerGas:    make([]byte, fieldparams.RootLength),
			BlockHash:        make([]byte, fieldparams.RootLength),
			TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
			ParentHash:       params.BeaconConfig().ZeroHash[:],
			Timestamp:        uint64(tiZond.Unix()),
			WithdrawalsRoot:  wr[:],
		},
		Pubkey: sk.PublicKey().Marshal(),
		Value:  bytesutil.PadTo([]byte{1, 2, 3}, 32),
	}
	srZond, err := signing.ComputeSigningRoot(bidZond, domain)
	require.NoError(t, err)
	sBidZond := &qrysmpb.SignedBuilderBidZond{
		Message:   bidZond,
		Signature: sk.Sign(srZond[:]).Marshal(),
	}

	require.NoError(t, err)
	tests := []struct {
		name               string
		head               interfaces.ReadOnlySignedBeaconBlock
		mock               *builderTest.MockBuilderService
		fetcher            *blockchainTest.ChainService
		err                string
		returnedHeaderZond *v1.ExecutionPayloadHeaderZond
	}{
		{
			name: "0 bid",
			mock: &builderTest.MockBuilderService{
				BidZond: &qrysmpb.SignedBuilderBidZond{
					Message: &qrysmpb.BuilderBidZond{
						Header: &v1.ExecutionPayloadHeaderZond{
							BlockNumber: 123,
						},
					},
				},
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
					require.NoError(t, err)
					wb.SetSlot(0)
					return wb
				}(),
			},
			err: "builder returned header with 0 bid amount",
		},
		{
			name: "invalid tx root",
			mock: &builderTest.MockBuilderService{
				BidZond: &qrysmpb.SignedBuilderBidZond{
					Message: &qrysmpb.BuilderBidZond{
						Value: []byte{1},
						Header: &v1.ExecutionPayloadHeaderZond{
							BlockNumber:      123,
							TransactionsRoot: emptyRoot[:],
						},
					},
				},
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
					require.NoError(t, err)
					wb.SetSlot(primitives.Slot(0))
					return wb
				}(),
			},
			err: "builder returned header with an empty tx root",
		},
		{
			name: "can get header",
			mock: &builderTest.MockBuilderService{
				BidZond: sBidZond,
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
					require.NoError(t, err)
					wb.SetSlot(0)
					return wb
				}(),
			},
			returnedHeaderZond: bidZond.Header,
		},
		// NOTE(rgeraldes24): test is not valid atm: re-enable once we have more versions
		/*
			{
				name: "wrong bid version",
				mock: &builderTest.MockBuilderService{
					BidZond: sBidZond,
				},
				fetcher: &blockchainTest.ChainService{
					Block: func() interfaces.ReadOnlySignedBeaconBlock {
						wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
						require.NoError(t, err)
						wb.SetSlot(0)
						return wb
					}(),
				},
				err: "is different from head block version",
			},
		*/
		{
			name: "different bid version during hard fork",
			mock: &builderTest.MockBuilderService{
				BidZond: sBidZond,
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
					require.NoError(t, err)
					wb.SetSlot(primitives.Slot(fakeZondEpoch) * params.BeaconConfig().SlotsPerEpoch)
					return wb
				}(),
			},
			returnedHeaderZond: bidZond.Header,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vs := &Server{BlockBuilder: tc.mock, HeadFetcher: tc.fetcher, TimeFetcher: &blockchainTest.ChainService{
				Genesis: genesis,
			}}
			hb, err := vs.HeadFetcher.HeadBlock(context.Background())
			require.NoError(t, err)
			h, err := vs.getPayloadHeaderFromBuilder(context.Background(), hb.Block().Slot(), 0, 0)
			if tc.err != "" {
				require.ErrorContains(t, tc.err, err)
			} else {
				require.NoError(t, err)
				if tc.returnedHeaderZond != nil {
					want, err := blocks.WrappedExecutionPayloadHeaderZond(tc.returnedHeaderZond, 0) // value is a mock
					require.NoError(t, err)
					require.DeepEqual(t, want, h)
				}
			}
		})
	}
}

func TestExpectedGasLimit(t *testing.T) {
	const maxGas = uint64(20_000_000) // qrlparams.MaxGasLimit
	const adj = uint64(1024)          // gasLimitAdjustmentFactor

	tests := []struct {
		name           string
		parentGasLimit uint64
		proposerGas    uint64
		want           uint64
	}{
		{
			name:           "proposer matches parent",
			parentGasLimit: maxGas,
			proposerGas:    maxGas,
			want:           maxGas,
		},
		{
			name:           "small upward step within bound",
			parentGasLimit: 10_000_000,
			proposerGas:    10_000_100,
			want:           10_000_100,
		},
		{
			name:           "upward step capped to parent + maxDiff",
			parentGasLimit: 10_000_000,
			proposerGas:    100_000_000,
			want:           10_000_000 + (10_000_000/adj - 1),
		},
		{
			name:           "downward step within bound",
			parentGasLimit: 10_000_000,
			proposerGas:    9_999_900,
			want:           9_999_900,
		},
		{
			name:           "downward step capped to parent - maxDiff",
			parentGasLimit: 10_000_000,
			proposerGas:    0,
			want:           10_000_000 - (10_000_000/adj - 1),
		},
		{
			name:           "proposer wants above MaxGasLimit, parent at cap: clamped to MaxGasLimit",
			parentGasLimit: maxGas,
			proposerGas:    maxGas * 10,
			want:           maxGas,
		},
		{
			name:           "proposer well above MaxGasLimit, parent below cap: clamped to MaxGasLimit",
			parentGasLimit: maxGas - 1, // very close to cap; parent+maxDiff would exceed
			proposerGas:    1 << 40,
			want:           maxGas, // would be parent + maxDiff = ~20_019_528, clamped down
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expectedGasLimit(tt.parentGasLimit, tt.proposerGas)
			require.Equal(t, tt.want, got)
			require.Equal(t, true, got <= 20_000_000, "result must never exceed MaxGasLimit")
		})
	}
}

func TestServer_validateBuilderSignature(t *testing.T) {
	sk, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	bid := &qrysmpb.BuilderBidZond{
		Header: &v1.ExecutionPayloadHeaderZond{
			ParentHash:       make([]byte, fieldparams.RootLength),
			FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:       make([]byte, fieldparams.RootLength),
			BaseFeePerGas:    make([]byte, fieldparams.RootLength),
			BlockHash:        make([]byte, fieldparams.RootLength),
			TransactionsRoot: make([]byte, fieldparams.RootLength),
			BlockNumber:      1,
			WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
		},
		Pubkey: sk.PublicKey().Marshal(),
		Value:  bytesutil.PadTo([]byte{1, 2, 3}, 32),
	}
	d := params.BeaconConfig().DomainApplicationBuilder
	domain, err := signing.ComputeDomain(d, nil, nil)
	require.NoError(t, err)
	sr, err := signing.ComputeSigningRoot(bid, domain)
	require.NoError(t, err)
	pbBid := &qrysmpb.SignedBuilderBidZond{
		Message:   bid,
		Signature: sk.Sign(sr[:]).Marshal(),
	}
	sBid, err := builder.WrappedSignedBuilderBidZond(pbBid)
	require.NoError(t, err)
	require.NoError(t, validateBuilderSignature(sBid))

	pbBid.Message.Value = make([]byte, 32)
	sBid, err = builder.WrappedSignedBuilderBidZond(pbBid)
	require.NoError(t, err)
	require.ErrorIs(t, validateBuilderSignature(sBid), signing.ErrSigFailedToVerify)
}

func Test_matchingWithdrawalsRoot(t *testing.T) {
	t.Run("could not get local withdrawals", func(t *testing.T) {
		local := &v1.ExecutionPayloadZond{}
		p, err := blocks.WrappedExecutionPayloadZond(local, 0)
		require.NoError(t, err)
		header := &v1.ExecutionPayloadHeaderZond{}
		h, err := blocks.WrappedExecutionPayloadHeaderZond(header, 0)
		require.NoError(t, err)
		_, err = matchingWithdrawalsRoot(h, p)
		require.ErrorContains(t, "could not get local withdrawals", err)
	})
	t.Run("could not get builder withdrawals root", func(t *testing.T) {
		local := &v1.ExecutionPayloadZond{}
		p, err := blocks.WrappedExecutionPayloadZond(local, 0)
		require.NoError(t, err)
		_, err = matchingWithdrawalsRoot(p, p)
		require.ErrorContains(t, "could not get builder withdrawals root", err)
	})
	t.Run("withdrawals mismatch", func(t *testing.T) {
		local := &v1.ExecutionPayloadZond{}
		p, err := blocks.WrappedExecutionPayloadZond(local, 0)
		require.NoError(t, err)
		header := &v1.ExecutionPayloadHeaderZond{}
		h, err := blocks.WrappedExecutionPayloadHeaderZond(header, 0)
		require.NoError(t, err)
		matched, err := matchingWithdrawalsRoot(p, h)
		require.NoError(t, err)
		require.Equal(t, false, matched)
	})
	t.Run("withdrawals match", func(t *testing.T) {
		wds := []*v1.Withdrawal{{
			Index:          1,
			ValidatorIndex: 2,
			Address:        make([]byte, fieldparams.FeeRecipientLength),
			Amount:         3,
		}}
		local := &v1.ExecutionPayloadZond{Withdrawals: wds}
		p, err := blocks.WrappedExecutionPayloadZond(local, 0)
		require.NoError(t, err)
		header := &v1.ExecutionPayloadHeaderZond{}
		wr, err := ssz.WithdrawalSliceRoot(wds, fieldparams.MaxWithdrawalsPerPayload)
		require.NoError(t, err)
		header.WithdrawalsRoot = wr[:]
		h, err := blocks.WrappedExecutionPayloadHeaderZond(header, 0)
		require.NoError(t, err)
		matched, err := matchingWithdrawalsRoot(p, h)
		require.NoError(t, err)
		require.Equal(t, true, matched)
	})
}

func TestEmptyTransactionsRoot(t *testing.T) {
	r, err := ssz.TransactionsRoot([][]byte{})
	require.NoError(t, err)
	require.DeepEqual(t, r, emptyTransactionsRoot)
}
