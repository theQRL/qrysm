package validator

import (
	"context"
	"math/big"
	"testing"
	"time"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/qrysm/api/client/builder"
	blockchainTest "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	builderTest "github.com/theQRL/qrysm/beacon-chain/builder/testing"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	dbTest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	powtesting "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	consensus_types "github.com/theQRL/qrysm/consensus-types"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/dilithium"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/encoding/ssz"
	v1 "github.com/theQRL/qrysm/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
)

func TestServer_setExecutionData(t *testing.T) {
	hook := logTest.NewGlobal()

	ctx := context.Background()
	params.SetupTestConfigCleanup(t)

	beaconDB := dbTest.SetupDB(t)
	capellaTransitionState, _ := util.DeterministicGenesisStateCapella(t, 1)
	wrappedHeaderCapella, err := blocks.WrappedExecutionPayloadHeaderCapella(&v1.ExecutionPayloadHeaderCapella{BlockNumber: 1}, 0)
	require.NoError(t, err)
	require.NoError(t, capellaTransitionState.SetLatestExecutionPayloadHeader(wrappedHeaderCapella))
	b2pbCapella := util.NewBeaconBlockCapella()
	b2rCapella, err := b2pbCapella.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b2pbCapella)
	require.NoError(t, capellaTransitionState.SetFinalizedCheckpoint(&zondpb.Checkpoint{
		Root: b2rCapella[:],
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
		ExecutionEngineCaller:  &powtesting.EngineClient{PayloadIDBytes: id, ExecutionPayloadCapella: &v1.ExecutionPayloadCapella{BlockNumber: 1, Withdrawals: withdrawals}, BlockValue: 0},
		HeadFetcher:            &blockchainTest.ChainService{State: capellaTransitionState},
		FinalizationFetcher:    &blockchainTest.ChainService{},
		BeaconDB:               beaconDB,
		ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
		BlockBuilder:           &builderTest.MockBuilderService{HasConfigured: true, Cfg: &builderTest.Config{BeaconDB: beaconDB}},
		ForkchoiceFetcher:      &blockchainTest.ChainService{},
	}

	t.Run("No builder configured. Use local block", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		b := blk.Block()
		localPayload, _, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderPayload, err := vs.getBuilderPayload(ctx, b.Slot(), b.ProposerIndex())
		require.NoError(t, err)
		require.NoError(t, setExecutionData(context.Background(), blk, localPayload, builderPayload))
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(1), e.BlockNumber()) // Local block
	})
	t.Run("Builder configured. Builder Block has higher value. Incorrect withdrawals", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		require.NoError(t, vs.BeaconDB.SaveRegistrationsByValidatorIDs(ctx, []primitives.ValidatorIndex{blk.Block().ProposerIndex()},
			[]*zondpb.ValidatorRegistrationV1{{FeeRecipient: make([]byte, fieldparams.FeeRecipientLength), Timestamp: uint64(time.Now().Unix()), Pubkey: make([]byte, field_params.DilithiumPubkeyLength)}}))
		ti, err := slots.ToTime(uint64(time.Now().Unix()), 0)
		require.NoError(t, err)
		sk, err := dilithium.RandKey()
		require.NoError(t, err)
		bid := &zondpb.BuilderBidCapella{
			Header: &v1.ExecutionPayloadHeaderCapella{
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
			},
			Pubkey: sk.PublicKey().Marshal(),
			Value:  bytesutil.PadTo([]byte{1}, 32),
		}
		d := params.BeaconConfig().DomainApplicationBuilder
		domain, err := signing.ComputeDomain(d, nil, nil)
		require.NoError(t, err)
		sr, err := signing.ComputeSigningRoot(bid, domain)
		require.NoError(t, err)
		sBid := &zondpb.SignedBuilderBidCapella{
			Message:   bid,
			Signature: sk.Sign(sr[:]).Marshal(),
		}
		vs.BlockBuilder = &builderTest.MockBuilderService{
			BidCapella:    sBid,
			HasConfigured: true,
			Cfg:           &builderTest.Config{BeaconDB: beaconDB},
		}
		wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		chain := &blockchainTest.ChainService{ForkChoiceStore: doublylinkedtree.New(), Genesis: time.Now(), Block: wb}
		vs.ForkchoiceFetcher = chain
		vs.ForkchoiceFetcher.SetForkChoiceGenesisTime(uint64(time.Now().Unix()))
		vs.TimeFetcher = chain
		vs.HeadFetcher = chain
		b := blk.Block()

		localPayload, _, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderPayload, err := vs.getBuilderPayload(ctx, b.Slot(), b.ProposerIndex())
		require.NoError(t, err)
		require.NoError(t, setExecutionData(context.Background(), blk, localPayload, builderPayload))
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(1), e.BlockNumber()) // Local block because incorrect withdrawals
	})
	t.Run("Builder configured. Builder Block has higher value. Correct withdrawals.", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBlindedBeaconBlockCapella())
		require.NoError(t, err)
		require.NoError(t, vs.BeaconDB.SaveRegistrationsByValidatorIDs(ctx, []primitives.ValidatorIndex{blk.Block().ProposerIndex()},
			[]*zondpb.ValidatorRegistrationV1{{FeeRecipient: make([]byte, fieldparams.FeeRecipientLength), Timestamp: uint64(time.Now().Unix()), Pubkey: make([]byte, field_params.DilithiumPubkeyLength)}}))
		ti, err := slots.ToTime(uint64(time.Now().Unix()), 0)
		require.NoError(t, err)
		sk, err := dilithium.RandKey()
		require.NoError(t, err)
		wr, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
		require.NoError(t, err)
		builderValue := bytesutil.ReverseByteOrder(big.NewInt(1e9).Bytes())
		bid := &zondpb.BuilderBidCapella{
			Header: &v1.ExecutionPayloadHeaderCapella{
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
			},
			Pubkey: sk.PublicKey().Marshal(),
			Value:  bytesutil.PadTo(builderValue, 32),
		}
		d := params.BeaconConfig().DomainApplicationBuilder
		domain, err := signing.ComputeDomain(d, nil, nil)
		require.NoError(t, err)
		sr, err := signing.ComputeSigningRoot(bid, domain)
		require.NoError(t, err)
		sBid := &zondpb.SignedBuilderBidCapella{
			Message:   bid,
			Signature: sk.Sign(sr[:]).Marshal(),
		}
		vs.BlockBuilder = &builderTest.MockBuilderService{
			BidCapella:    sBid,
			HasConfigured: true,
			Cfg:           &builderTest.Config{BeaconDB: beaconDB},
		}
		wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		chain := &blockchainTest.ChainService{ForkChoiceStore: doublylinkedtree.New(), Genesis: time.Now(), Block: wb}
		vs.ForkFetcher = chain
		vs.ForkchoiceFetcher.SetForkChoiceGenesisTime(uint64(time.Now().Unix()))
		vs.TimeFetcher = chain
		vs.HeadFetcher = chain

		b := blk.Block()
		localPayload, _, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderPayload, err := vs.getBuilderPayload(ctx, b.Slot(), b.ProposerIndex())
		require.NoError(t, err)
		require.NoError(t, setExecutionData(context.Background(), blk, localPayload, builderPayload))
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(2), e.BlockNumber()) // Builder block
	})
	t.Run("Builder configured. Local block has higher value", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		vs.ExecutionEngineCaller = &powtesting.EngineClient{PayloadIDBytes: id, ExecutionPayloadCapella: &v1.ExecutionPayloadCapella{BlockNumber: 3}, BlockValue: 2}
		b := blk.Block()
		localPayload, _, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		require.NoError(t, err)
		builderPayload, err := vs.getBuilderPayload(ctx, b.Slot(), b.ProposerIndex())
		require.NoError(t, err)
		require.NoError(t, setExecutionData(context.Background(), blk, localPayload, builderPayload))
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(3), e.BlockNumber()) // Local block

		require.LogsContain(t, hook, "builderGweiValue=1 localBoostPercentage=0 localGweiValue=2")
	})
	t.Run("Builder configured. Local block and boost has higher value", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.LocalBlockValueBoost = 1 // Boost 1%.
		params.OverrideBeaconConfig(cfg)

		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		vs.ExecutionEngineCaller = &powtesting.EngineClient{PayloadIDBytes: id, ExecutionPayloadCapella: &v1.ExecutionPayloadCapella{BlockNumber: 3}, BlockValue: 1}
		b := blk.Block()
		localPayload, _, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderPayload, err := vs.getBuilderPayload(ctx, b.Slot(), b.ProposerIndex())
		require.NoError(t, err)
		require.NoError(t, setExecutionData(context.Background(), blk, localPayload, builderPayload))
		e, err := blk.Block().Body().Execution()
		require.NoError(t, err)
		require.Equal(t, uint64(3), e.BlockNumber()) // Local block

		require.LogsContain(t, hook, "builderGweiValue=1 localBoostPercentage=1 localGweiValue=1")
	})
	t.Run("Builder configured. Builder returns fault. Use local block", func(t *testing.T) {
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		vs.BlockBuilder = &builderTest.MockBuilderService{
			// ErrGetHeader:  errors.New("fault"),
			HasConfigured: true,
			Cfg:           &builderTest.Config{BeaconDB: beaconDB},
		}
		vs.ExecutionEngineCaller = &powtesting.EngineClient{PayloadIDBytes: id, ExecutionPayloadCapella: &v1.ExecutionPayloadCapella{BlockNumber: 4}, BlockValue: 0}
		b := blk.Block()
		localPayload, _, err := vs.getLocalPayload(ctx, b, capellaTransitionState)
		require.NoError(t, err)
		builderPayload, err := vs.getBuilderPayload(ctx, b.Slot(), b.ProposerIndex())
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
	fakeCapellaEpoch := primitives.Epoch(0)
	emptyRoot, err := ssz.TransactionsRoot([][]byte{})
	require.NoError(t, err)

	sk, err := dilithium.RandKey()
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

	tiCapella, err := slots.ToTime(uint64(genesis.Unix()), primitives.Slot(fakeCapellaEpoch)*params.BeaconConfig().SlotsPerEpoch)
	require.NoError(t, err)
	bidCapella := &zondpb.BuilderBidCapella{
		Header: &v1.ExecutionPayloadHeaderCapella{
			FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:        make([]byte, fieldparams.RootLength),
			ReceiptsRoot:     make([]byte, fieldparams.RootLength),
			LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:       make([]byte, fieldparams.RootLength),
			BaseFeePerGas:    make([]byte, fieldparams.RootLength),
			BlockHash:        make([]byte, fieldparams.RootLength),
			TransactionsRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
			ParentHash:       params.BeaconConfig().ZeroHash[:],
			Timestamp:        uint64(tiCapella.Unix()),
			WithdrawalsRoot:  wr[:],
		},
		Pubkey: sk.PublicKey().Marshal(),
		Value:  bytesutil.PadTo([]byte{1, 2, 3}, 32),
	}
	srCapella, err := signing.ComputeSigningRoot(bidCapella, domain)
	require.NoError(t, err)
	sBidCapella := &zondpb.SignedBuilderBidCapella{
		Message:   bidCapella,
		Signature: sk.Sign(srCapella[:]).Marshal(),
	}

	require.NoError(t, err)
	tests := []struct {
		name                  string
		head                  interfaces.ReadOnlySignedBeaconBlock
		mock                  *builderTest.MockBuilderService
		fetcher               *blockchainTest.ChainService
		err                   string
		returnedHeaderCapella *v1.ExecutionPayloadHeaderCapella
	}{
		{
			name: "0 bid",
			mock: &builderTest.MockBuilderService{
				BidCapella: &zondpb.SignedBuilderBidCapella{
					Message: &zondpb.BuilderBidCapella{
						Header: &v1.ExecutionPayloadHeaderCapella{
							BlockNumber: 123,
						},
					},
				},
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
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
				BidCapella: &zondpb.SignedBuilderBidCapella{
					Message: &zondpb.BuilderBidCapella{
						Value: []byte{1},
						Header: &v1.ExecutionPayloadHeaderCapella{
							BlockNumber:      123,
							TransactionsRoot: emptyRoot[:],
						},
					},
				},
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
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
				BidCapella: sBidCapella,
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
					require.NoError(t, err)
					wb.SetSlot(0)
					return wb
				}(),
			},
			returnedHeaderCapella: bidCapella.Header,
		},
		// NOTE(rgeraldes24): test is not valid atm: re-enable once we have more versions
		/*
			{
				name: "wrong bid version",
				mock: &builderTest.MockBuilderService{
					BidCapella: sBidCapella,
				},
				fetcher: &blockchainTest.ChainService{
					Block: func() interfaces.ReadOnlySignedBeaconBlock {
						wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
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
				BidCapella: sBidCapella,
			},
			fetcher: &blockchainTest.ChainService{
				Block: func() interfaces.ReadOnlySignedBeaconBlock {
					wb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
					require.NoError(t, err)
					wb.SetSlot(primitives.Slot(fakeCapellaEpoch) * params.BeaconConfig().SlotsPerEpoch)
					return wb
				}(),
			},
			returnedHeaderCapella: bidCapella.Header,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vs := &Server{BlockBuilder: tc.mock, HeadFetcher: tc.fetcher, TimeFetcher: &blockchainTest.ChainService{
				Genesis: genesis,
			}}
			hb, err := vs.HeadFetcher.HeadBlock(context.Background())
			require.NoError(t, err)
			h, err := vs.getPayloadHeaderFromBuilder(context.Background(), hb.Block().Slot(), 0)
			if tc.err != "" {
				require.ErrorContains(t, tc.err, err)
			} else {
				require.NoError(t, err)
				if tc.returnedHeaderCapella != nil {
					want, err := blocks.WrappedExecutionPayloadHeaderCapella(tc.returnedHeaderCapella, 0) // value is a mock
					require.NoError(t, err)
					require.DeepEqual(t, want, h)
				}
			}
		})
	}
}

func TestServer_validateBuilderSignature(t *testing.T) {
	sk, err := dilithium.RandKey()
	require.NoError(t, err)
	bid := &zondpb.BuilderBidCapella{
		Header: &v1.ExecutionPayloadHeaderCapella{
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
	pbBid := &zondpb.SignedBuilderBidCapella{
		Message:   bid,
		Signature: sk.Sign(sr[:]).Marshal(),
	}
	sBid, err := builder.WrappedSignedBuilderBidCapella(pbBid)
	require.NoError(t, err)
	require.NoError(t, validateBuilderSignature(sBid))

	pbBid.Message.Value = make([]byte, 32)
	sBid, err = builder.WrappedSignedBuilderBidCapella(pbBid)
	require.NoError(t, err)
	require.ErrorIs(t, validateBuilderSignature(sBid), signing.ErrSigFailedToVerify)
}

func Test_matchingWithdrawalsRoot(t *testing.T) {
	t.Run("could not get local withdrawals", func(t *testing.T) {
		local := &v1.ExecutionPayloadCapella{}
		p, err := blocks.WrappedExecutionPayloadCapella(local, 0)
		require.NoError(t, err)
		header := &v1.ExecutionPayloadHeaderCapella{}
		h, err := blocks.WrappedExecutionPayloadHeaderCapella(header, 0)
		require.NoError(t, err)
		_, err = matchingWithdrawalsRoot(h, p)
		require.ErrorContains(t, "could not get local withdrawals", err)
	})
	t.Run("could not get builder withdrawals root", func(t *testing.T) {
		local := &v1.ExecutionPayloadCapella{}
		p, err := blocks.WrappedExecutionPayloadCapella(local, 0)
		require.NoError(t, err)
		_, err = matchingWithdrawalsRoot(p, p)
		require.ErrorContains(t, "could not get builder withdrawals root", err)
	})
	t.Run("withdrawals mismatch", func(t *testing.T) {
		local := &v1.ExecutionPayloadCapella{}
		p, err := blocks.WrappedExecutionPayloadCapella(local, 0)
		require.NoError(t, err)
		header := &v1.ExecutionPayloadHeaderCapella{}
		h, err := blocks.WrappedExecutionPayloadHeaderCapella(header, 0)
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
		local := &v1.ExecutionPayloadCapella{Withdrawals: wds}
		p, err := blocks.WrappedExecutionPayloadCapella(local, 0)
		require.NoError(t, err)
		header := &v1.ExecutionPayloadHeaderCapella{}
		wr, err := ssz.WithdrawalSliceRoot(wds, fieldparams.MaxWithdrawalsPerPayload)
		require.NoError(t, err)
		header.WithdrawalsRoot = wr[:]
		h, err := blocks.WrappedExecutionPayloadHeaderCapella(header, 0)
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
