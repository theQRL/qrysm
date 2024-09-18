package beacon

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/theQRL/go-bitfield"
	chainMock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	dbTest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/operations/attestations"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/cmd"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
	"google.golang.org/protobuf/proto"
)

func TestServer_ListAttestations_NoResults(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	st, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Slot: 0,
	})
	require.NoError(t, err)
	bs := &Server{
		BeaconDB: db,
		HeadFetcher: &chainMock.ChainService{
			State: st,
		},
	}
	wanted := &zondpb.ListAttestationsResponse{
		Attestations:  make([]*zondpb.Attestation, 0),
		TotalSize:     int32(0),
		NextPageToken: strconv.Itoa(0),
	}
	res, err := bs.ListAttestations(ctx, &zondpb.ListAttestationsRequest{
		QueryFilter: &zondpb.ListAttestationsRequest_GenesisEpoch{GenesisEpoch: true},
	})
	require.NoError(t, err)
	if !proto.Equal(wanted, res) {
		t.Errorf("Wanted %v, received %v", wanted, res)
	}
}

func TestServer_ListAttestations_Genesis(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	st, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Slot: 0,
	})
	require.NoError(t, err)
	bs := &Server{
		BeaconDB: db,
		HeadFetcher: &chainMock.ChainService{
			State: st,
		},
	}

	att := util.HydrateAttestation(&zondpb.Attestation{
		AggregationBits: bitfield.NewBitlist(0),
		Data: &zondpb.AttestationData{
			Slot:           2,
			CommitteeIndex: 1,
		},
	})

	parentRoot := [32]byte{1, 2, 3}
	signedBlock := util.NewBeaconBlockCapella()
	signedBlock.Block.ParentRoot = bytesutil.PadTo(parentRoot[:], 32)
	signedBlock.Block.Body.Attestations = []*zondpb.Attestation{att}
	root, err := signedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, db, signedBlock)
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))
	wanted := &zondpb.ListAttestationsResponse{
		Attestations:  []*zondpb.Attestation{att},
		NextPageToken: "",
		TotalSize:     1,
	}

	res, err := bs.ListAttestations(ctx, &zondpb.ListAttestationsRequest{
		QueryFilter: &zondpb.ListAttestationsRequest_GenesisEpoch{
			GenesisEpoch: true,
		},
	})
	require.NoError(t, err)
	require.DeepSSZEqual(t, wanted, res)
}

func TestServer_ListAttestations_NoPagination(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	count := primitives.Slot(8)
	atts := make([]*zondpb.Attestation, 0, count)
	for i := primitives.Slot(0); i < count; i++ {
		blockExample := util.NewBeaconBlockCapella()
		blockExample.Block.Body.Attestations = []*zondpb.Attestation{
			{
				Signatures: [][]byte{make([]byte, field_params.DilithiumSignatureLength)},
				Data: &zondpb.AttestationData{
					Target:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte("root"), 32)},
					Source:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte("root"), 32)},
					BeaconBlockRoot: bytesutil.PadTo([]byte("root"), 32),
					Slot:            i,
				},
				AggregationBits: bitfield.Bitlist{0b11},
			},
		}
		util.SaveBlock(t, ctx, db, blockExample)
		atts = append(atts, blockExample.Block.Body.Attestations...)
	}

	bs := &Server{
		BeaconDB: db,
	}

	received, err := bs.ListAttestations(ctx, &zondpb.ListAttestationsRequest{
		QueryFilter: &zondpb.ListAttestationsRequest_GenesisEpoch{
			GenesisEpoch: true,
		},
	})
	require.NoError(t, err)
	require.DeepEqual(t, atts, received.Attestations, "Incorrect attestations response")
}

func TestServer_ListAttestations_FiltersCorrectly(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	someRoot := [32]byte{1, 2, 3}
	sourceRoot := [32]byte{4, 5, 6}
	sourceEpoch := primitives.Epoch(5)
	targetRoot := [32]byte{7, 8, 9}
	targetEpoch := primitives.Epoch(7)

	unwrappedBlocks := []*zondpb.SignedBeaconBlockCapella{
		util.HydrateSignedBeaconBlockCapella(
			&zondpb.SignedBeaconBlockCapella{
				Block: &zondpb.BeaconBlockCapella{
					Slot: 4,
					Body: &zondpb.BeaconBlockBodyCapella{
						Attestations: []*zondpb.Attestation{
							{
								Data: &zondpb.AttestationData{
									BeaconBlockRoot: someRoot[:],
									Source: &zondpb.Checkpoint{
										Root:  sourceRoot[:],
										Epoch: sourceEpoch,
									},
									Target: &zondpb.Checkpoint{
										Root:  targetRoot[:],
										Epoch: targetEpoch,
									},
									Slot: 3,
								},
								AggregationBits: bitfield.Bitlist{0b11},
								Signatures:      [][]byte{bytesutil.PadTo([]byte("sig"), fieldparams.DilithiumSignatureLength)},
							},
						},
					},
				},
			}),
		util.HydrateSignedBeaconBlockCapella(&zondpb.SignedBeaconBlockCapella{
			Block: &zondpb.BeaconBlockCapella{
				Slot: 5 + params.BeaconConfig().SlotsPerEpoch,
				Body: &zondpb.BeaconBlockBodyCapella{
					Attestations: []*zondpb.Attestation{
						{
							Data: &zondpb.AttestationData{
								BeaconBlockRoot: someRoot[:],
								Source: &zondpb.Checkpoint{
									Root:  sourceRoot[:],
									Epoch: sourceEpoch,
								},
								Target: &zondpb.Checkpoint{
									Root:  targetRoot[:],
									Epoch: targetEpoch,
								},
								Slot: 4 + params.BeaconConfig().SlotsPerEpoch,
							},
							AggregationBits: bitfield.Bitlist{0b11},
							Signatures:      [][]byte{bytesutil.PadTo([]byte("sig"), fieldparams.DilithiumSignatureLength)},
						},
					},
				},
			},
		}),
		util.HydrateSignedBeaconBlockCapella(
			&zondpb.SignedBeaconBlockCapella{
				Block: &zondpb.BeaconBlockCapella{
					Slot: 5,
					Body: &zondpb.BeaconBlockBodyCapella{
						Attestations: []*zondpb.Attestation{
							{
								Data: &zondpb.AttestationData{
									BeaconBlockRoot: someRoot[:],
									Source: &zondpb.Checkpoint{
										Root:  sourceRoot[:],
										Epoch: sourceEpoch,
									},
									Target: &zondpb.Checkpoint{
										Root:  targetRoot[:],
										Epoch: targetEpoch,
									},
									Slot: 4,
								},
								AggregationBits: bitfield.Bitlist{0b11},
								Signatures:      [][]byte{bytesutil.PadTo([]byte("sig"), fieldparams.DilithiumSignatureLength)},
							},
						},
					},
				},
			}),
	}

	var blocks []interfaces.ReadOnlySignedBeaconBlock
	for _, b := range unwrappedBlocks {
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		blocks = append(blocks, wsb)
	}

	require.NoError(t, db.SaveBlocks(ctx, blocks))

	bs := &Server{
		BeaconDB: db,
	}

	received, err := bs.ListAttestations(ctx, &zondpb.ListAttestationsRequest{
		QueryFilter: &zondpb.ListAttestationsRequest_Epoch{Epoch: 1},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, len(received.Attestations))
	received, err = bs.ListAttestations(ctx, &zondpb.ListAttestationsRequest{
		QueryFilter: &zondpb.ListAttestationsRequest_GenesisEpoch{GenesisEpoch: true},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, len(received.Attestations))
}

func TestServer_ListAttestations_Pagination_CustomPageParameters(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	count := params.BeaconConfig().SlotsPerEpoch * 4
	atts := make([]*zondpb.Attestation, 0, count)
	for i := primitives.Slot(0); i < params.BeaconConfig().SlotsPerEpoch; i++ {
		for s := primitives.CommitteeIndex(0); s < 4; s++ {
			blockExample := util.NewBeaconBlockCapella()
			blockExample.Block.Slot = i
			blockExample.Block.Body.Attestations = []*zondpb.Attestation{
				util.HydrateAttestation(&zondpb.Attestation{
					Data: &zondpb.AttestationData{
						CommitteeIndex: s,
						Slot:           i,
					},
					AggregationBits: bitfield.Bitlist{0b11},
				}),
			}
			util.SaveBlock(t, ctx, db, blockExample)
			atts = append(atts, blockExample.Block.Body.Attestations...)
		}
	}
	sort.Sort(sortableAttestations(atts))

	bs := &Server{
		BeaconDB: db,
	}

	tests := []struct {
		name string
		req  *zondpb.ListAttestationsRequest
		res  *zondpb.ListAttestationsResponse
	}{
		{
			name: "1st of 3 pages",
			req: &zondpb.ListAttestationsRequest{
				QueryFilter: &zondpb.ListAttestationsRequest_GenesisEpoch{
					GenesisEpoch: true,
				},
				PageToken: strconv.Itoa(1),
				PageSize:  3,
			},
			res: &zondpb.ListAttestationsResponse{
				Attestations: []*zondpb.Attestation{
					atts[3],
					atts[4],
					atts[5],
				},
				NextPageToken: strconv.Itoa(2),
				TotalSize:     int32(count),
			},
		},
		{
			name: "10 of size 1",
			req: &zondpb.ListAttestationsRequest{
				QueryFilter: &zondpb.ListAttestationsRequest_GenesisEpoch{
					GenesisEpoch: true,
				},
				PageToken: strconv.Itoa(10),
				PageSize:  1,
			},
			res: &zondpb.ListAttestationsResponse{
				Attestations: []*zondpb.Attestation{
					atts[10],
				},
				NextPageToken: strconv.Itoa(11),
				TotalSize:     int32(count),
			},
		},
		{
			name: "2 of size 8",
			req: &zondpb.ListAttestationsRequest{
				QueryFilter: &zondpb.ListAttestationsRequest_GenesisEpoch{
					GenesisEpoch: true,
				},
				PageToken: strconv.Itoa(2),
				PageSize:  8,
			},
			res: &zondpb.ListAttestationsResponse{
				Attestations: []*zondpb.Attestation{
					atts[16],
					atts[17],
					atts[18],
					atts[19],
					atts[20],
					atts[21],
					atts[22],
					atts[23],
				},
				NextPageToken: strconv.Itoa(3),
				TotalSize:     int32(count)},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, err := bs.ListAttestations(ctx, test.req)
			require.NoError(t, err)
			require.DeepSSZEqual(t, res, test.res)
		})
	}
}

func TestServer_ListAttestations_Pagination_OutOfRange(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()
	util.NewBeaconBlockCapella()
	count := primitives.Slot(1)
	atts := make([]*zondpb.Attestation, 0, count)
	for i := primitives.Slot(0); i < count; i++ {
		blockExample := util.HydrateSignedBeaconBlockCapella(&zondpb.SignedBeaconBlockCapella{
			Block: &zondpb.BeaconBlockCapella{
				Body: &zondpb.BeaconBlockBodyCapella{
					Attestations: []*zondpb.Attestation{
						{
							Data: &zondpb.AttestationData{
								BeaconBlockRoot: bytesutil.PadTo([]byte("root"), fieldparams.RootLength),
								Source:          &zondpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
								Target:          &zondpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
								Slot:            i,
							},
							AggregationBits: bitfield.Bitlist{0b11},
							Signatures:      [][]byte{make([]byte, fieldparams.DilithiumSignatureLength)},
						},
					},
				},
			},
		})
		util.SaveBlock(t, ctx, db, blockExample)
		atts = append(atts, blockExample.Block.Body.Attestations...)
	}

	bs := &Server{
		BeaconDB: db,
	}

	req := &zondpb.ListAttestationsRequest{
		QueryFilter: &zondpb.ListAttestationsRequest_Epoch{
			Epoch: 0,
		},
		PageToken: strconv.Itoa(1),
		PageSize:  100,
	}
	wanted := fmt.Sprintf("page start %d >= list %d", req.PageSize, len(atts))
	_, err := bs.ListAttestations(ctx, req)
	assert.ErrorContains(t, wanted, err)
}

func TestServer_ListAttestations_Pagination_ExceedsMaxPageSize(t *testing.T) {
	ctx := context.Background()
	bs := &Server{}
	exceedsMax := int32(cmd.Get().MaxRPCPageSize + 1)

	wanted := fmt.Sprintf("Requested page size %d can not be greater than max size %d", exceedsMax, cmd.Get().MaxRPCPageSize)
	req := &zondpb.ListAttestationsRequest{PageToken: strconv.Itoa(0), PageSize: exceedsMax}
	_, err := bs.ListAttestations(ctx, req)
	assert.ErrorContains(t, wanted, err)
}

func TestServer_ListAttestations_Pagination_DefaultPageSize(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	count := primitives.Slot(params.BeaconConfig().DefaultPageSize)
	atts := make([]*zondpb.Attestation, 0, count)
	for i := primitives.Slot(0); i < count; i++ {
		blockExample := util.NewBeaconBlockCapella()
		blockExample.Block.Body.Attestations = []*zondpb.Attestation{
			{
				Data: &zondpb.AttestationData{
					BeaconBlockRoot: bytesutil.PadTo([]byte("root"), 32),
					Target:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte("root"), 32)},
					Source:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte("root"), 32)},
					Slot:            i,
				},
				Signatures:      [][]byte{bytesutil.PadTo([]byte("root"), fieldparams.DilithiumSignatureLength)},
				AggregationBits: bitfield.Bitlist{0b11},
			},
		}
		util.SaveBlock(t, ctx, db, blockExample)
		atts = append(atts, blockExample.Block.Body.Attestations...)
	}

	bs := &Server{
		BeaconDB: db,
	}

	req := &zondpb.ListAttestationsRequest{
		QueryFilter: &zondpb.ListAttestationsRequest_GenesisEpoch{
			GenesisEpoch: true,
		},
	}
	res, err := bs.ListAttestations(ctx, req)
	require.NoError(t, err)

	i := 0
	j := params.BeaconConfig().DefaultPageSize
	assert.DeepEqual(t, atts[i:j], res.Attestations, "Incorrect attestations response")
}

func TestServer_mapAttestationToTargetRoot(t *testing.T) {
	count := primitives.Slot(100)
	atts := make([]*zondpb.Attestation, count)
	targetRoot1 := bytesutil.ToBytes32([]byte("root1"))
	targetRoot2 := bytesutil.ToBytes32([]byte("root2"))

	for i := primitives.Slot(0); i < count; i++ {
		var targetRoot [32]byte
		if i%2 == 0 {
			targetRoot = targetRoot1
		} else {
			targetRoot = targetRoot2
		}
		atts[i] = &zondpb.Attestation{
			Data: &zondpb.AttestationData{
				Target: &zondpb.Checkpoint{
					Root: targetRoot[:],
				},
			},
			AggregationBits: bitfield.Bitlist{0b11},
		}

	}
	mappedAtts := mapAttestationsByTargetRoot(atts)
	wantedMapLen := 2
	wantedMapNumberOfElements := 50
	assert.Equal(t, wantedMapLen, len(mappedAtts), "Unexpected mapped attestations length")
	assert.Equal(t, wantedMapNumberOfElements, len(mappedAtts[targetRoot1]), "Unexpected number of attestations per block root")
	assert.Equal(t, wantedMapNumberOfElements, len(mappedAtts[targetRoot2]), "Unexpected number of attestations per block root")
}

func TestServer_ListIndexedAttestations_GenesisEpoch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.BeaconConfig())
	db := dbTest.SetupDB(t)
	helpers.ClearCache()
	ctx := context.Background()
	targetRoot1 := bytesutil.ToBytes32([]byte("root"))
	targetRoot2 := bytesutil.ToBytes32([]byte("root2"))

	count := params.BeaconConfig().SlotsPerEpoch
	atts := make([]*zondpb.Attestation, 0, count)
	atts2 := make([]*zondpb.Attestation, 0, count)

	for i := primitives.Slot(0); i < count; i++ {
		var targetRoot [32]byte
		if i%2 == 0 {
			targetRoot = targetRoot1
		} else {
			targetRoot = targetRoot2
		}
		blockExample := util.NewBeaconBlockCapella()
		blockExample.Block.Body.Attestations = []*zondpb.Attestation{
			{
				Signatures: [][]byte{},
				Data: &zondpb.AttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
					Target: &zondpb.Checkpoint{
						Root: targetRoot[:],
					},
					Source: &zondpb.Checkpoint{
						Root: make([]byte, fieldparams.RootLength),
					},
					Slot:           i,
					CommitteeIndex: 0,
				},
				AggregationBits: bitfield.NewBitlist(128 / uint64(params.BeaconConfig().SlotsPerEpoch)),
			},
		}
		util.SaveBlock(t, ctx, db, blockExample)
		if i%2 == 0 {
			atts = append(atts, blockExample.Block.Body.Attestations...)
		} else {
			atts2 = append(atts2, blockExample.Block.Body.Attestations...)
		}

	}

	// We setup 512 validators so that committee size matches the length of attestations' aggregation bits.
	numValidators := uint64(512)
	state, _ := util.DeterministicGenesisStateCapella(t, numValidators)

	// Next up we convert the test attestations to indexed form:
	indexedAtts := make([]*zondpb.IndexedAttestation, len(atts)+len(atts2))
	for i := 0; i < len(atts); i++ {
		att := atts[i]
		committee, err := helpers.BeaconCommitteeFromState(context.Background(), state, att.Data.Slot, att.Data.CommitteeIndex)
		require.NoError(t, err)
		idxAtt, err := attestation.ConvertToIndexed(ctx, atts[i], committee)
		require.NoError(t, err, "Could not convert attestation to indexed")
		indexedAtts[i] = idxAtt
	}
	for i := 0; i < len(atts2); i++ {
		att := atts2[i]
		committee, err := helpers.BeaconCommitteeFromState(context.Background(), state, att.Data.Slot, att.Data.CommitteeIndex)
		require.NoError(t, err)
		idxAtt, err := attestation.ConvertToIndexed(ctx, atts2[i], committee)
		require.NoError(t, err, "Could not convert attestation to indexed")
		indexedAtts[i+len(atts)] = idxAtt
	}

	bs := &Server{
		BeaconDB:           db,
		GenesisTimeFetcher: &chainMock.ChainService{State: state},
		HeadFetcher:        &chainMock.ChainService{State: state},
		StateGen:           stategen.New(db, doublylinkedtree.New()),
	}
	err := db.SaveStateSummary(ctx, &zondpb.StateSummary{
		Root: targetRoot1[:],
		Slot: 1,
	})
	require.NoError(t, err)

	err = db.SaveStateSummary(ctx, &zondpb.StateSummary{
		Root: targetRoot2[:],
		Slot: 2,
	})
	require.NoError(t, err)

	require.NoError(t, db.SaveState(ctx, state, bytesutil.ToBytes32(targetRoot1[:])))
	require.NoError(t, state.SetSlot(state.Slot()+1))
	require.NoError(t, db.SaveState(ctx, state, bytesutil.ToBytes32(targetRoot2[:])))
	res, err := bs.ListIndexedAttestations(ctx, &zondpb.ListIndexedAttestationsRequest{
		QueryFilter: &zondpb.ListIndexedAttestationsRequest_GenesisEpoch{
			GenesisEpoch: true,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, len(indexedAtts), len(res.IndexedAttestations), "Incorrect indexted attestations length")
	sort.Slice(indexedAtts, func(i, j int) bool {
		return indexedAtts[i].Data.Slot < indexedAtts[j].Data.Slot
	})
	sort.Slice(res.IndexedAttestations, func(i, j int) bool {
		return res.IndexedAttestations[i].Data.Slot < res.IndexedAttestations[j].Data.Slot
	})

	assert.DeepEqual(t, indexedAtts, res.IndexedAttestations, "Incorrect list indexed attestations response")
}

func TestServer_ListIndexedAttestations_OldEpoch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.BeaconConfig())
	db := dbTest.SetupDB(t)
	helpers.ClearCache()
	ctx := context.Background()

	blockRoot := bytesutil.ToBytes32([]byte("root"))
	count := params.BeaconConfig().SlotsPerEpoch
	atts := make([]*zondpb.Attestation, 0, count)
	epoch := primitives.Epoch(50)
	startSlot, err := slots.EpochStart(epoch)
	require.NoError(t, err)

	for i := startSlot; i < count; i++ {
		blockExample := &zondpb.SignedBeaconBlockCapella{
			Block: &zondpb.BeaconBlockCapella{
				Body: &zondpb.BeaconBlockBodyCapella{
					Attestations: []*zondpb.Attestation{
						{
							Data: &zondpb.AttestationData{
								BeaconBlockRoot: blockRoot[:],
								Slot:            i,
								CommitteeIndex:  0,
								Target: &zondpb.Checkpoint{
									Epoch: epoch,
									Root:  make([]byte, fieldparams.RootLength),
								},
							},
							AggregationBits: bitfield.Bitlist{0b11},
						},
					},
				},
			},
		}
		util.SaveBlock(t, ctx, db, blockExample)
		atts = append(atts, blockExample.Block.Body.Attestations...)
	}

	// We setup 128 validators.
	numValidators := uint64(128)
	state, _ := util.DeterministicGenesisStateCapella(t, numValidators)

	randaoMixes := make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector)
	for i := 0; i < len(randaoMixes); i++ {
		randaoMixes[i] = make([]byte, fieldparams.RootLength)
	}
	require.NoError(t, state.SetRandaoMixes(randaoMixes))
	require.NoError(t, state.SetSlot(startSlot))

	// Next up we convert the test attestations to indexed form:
	indexedAtts := make([]*zondpb.IndexedAttestation, len(atts))
	for i := 0; i < len(atts); i++ {
		att := atts[i]
		committee, err := helpers.BeaconCommitteeFromState(context.Background(), state, att.Data.Slot, att.Data.CommitteeIndex)
		require.NoError(t, err)
		idxAtt, err := attestation.ConvertToIndexed(ctx, atts[i], committee)
		require.NoError(t, err, "Could not convert attestation to indexed")
		indexedAtts[i] = idxAtt
	}

	bs := &Server{
		BeaconDB: db,
		GenesisTimeFetcher: &chainMock.ChainService{
			Genesis: time.Now(),
		},
		StateGen: stategen.New(db, doublylinkedtree.New()),
	}
	err = db.SaveStateSummary(ctx, &zondpb.StateSummary{
		Root: blockRoot[:],
		Slot: params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epoch)),
	})
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, state, bytesutil.ToBytes32([]byte("root"))))
	res, err := bs.ListIndexedAttestations(ctx, &zondpb.ListIndexedAttestationsRequest{
		QueryFilter: &zondpb.ListIndexedAttestationsRequest_Epoch{
			Epoch: epoch,
		},
	})
	require.NoError(t, err)
	require.DeepEqual(t, indexedAtts, res.IndexedAttestations, "Incorrect list indexed attestations response")
}

func TestServer_AttestationPool_Pagination_ExceedsMaxPageSize(t *testing.T) {
	ctx := context.Background()
	bs := &Server{}
	exceedsMax := int32(cmd.Get().MaxRPCPageSize + 1)

	wanted := fmt.Sprintf("Requested page size %d can not be greater than max size %d", exceedsMax, cmd.Get().MaxRPCPageSize)
	req := &zondpb.AttestationPoolRequest{PageToken: strconv.Itoa(0), PageSize: exceedsMax}
	_, err := bs.AttestationPool(ctx, req)
	assert.ErrorContains(t, wanted, err)
}

func TestServer_AttestationPool_Pagination_OutOfRange(t *testing.T) {
	ctx := context.Background()
	bs := &Server{
		AttestationsPool: attestations.NewPool(),
	}

	atts := []*zondpb.Attestation{
		{
			Data: &zondpb.AttestationData{
				Slot:            1,
				BeaconBlockRoot: bytesutil.PadTo([]byte{1}, 32),
				Source:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte{1}, 32)},
				Target:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte{1}, 32)},
			},
			AggregationBits: bitfield.Bitlist{0b1101},
			Signatures:      [][]byte{bytesutil.PadTo([]byte{1}, fieldparams.DilithiumSignatureLength), bytesutil.PadTo([]byte{1}, fieldparams.DilithiumSignatureLength)},
		},
		{
			Data: &zondpb.AttestationData{
				Slot:            2,
				BeaconBlockRoot: bytesutil.PadTo([]byte{2}, 32),
				Source:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte{2}, 32)},
				Target:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte{2}, 32)},
			},
			AggregationBits: bitfield.Bitlist{0b1101},
			Signatures:      [][]byte{bytesutil.PadTo([]byte{2}, fieldparams.DilithiumSignatureLength), bytesutil.PadTo([]byte{2}, fieldparams.DilithiumSignatureLength)},
		},
		{
			Data: &zondpb.AttestationData{
				Slot:            3,
				BeaconBlockRoot: bytesutil.PadTo([]byte{3}, 32),
				Source:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte{3}, 32)},
				Target:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte{3}, 32)},
			},
			AggregationBits: bitfield.Bitlist{0b1101},
			Signatures:      [][]byte{bytesutil.PadTo([]byte{3}, fieldparams.DilithiumSignatureLength), bytesutil.PadTo([]byte{3}, fieldparams.DilithiumSignatureLength)},
		},
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))

	req := &zondpb.AttestationPoolRequest{
		PageToken: strconv.Itoa(1),
		PageSize:  100,
	}
	wanted := fmt.Sprintf("page start %d >= list %d", req.PageSize, len(atts))
	_, err := bs.AttestationPool(ctx, req)
	assert.ErrorContains(t, wanted, err)
}

func TestServer_AttestationPool_Pagination_DefaultPageSize(t *testing.T) {
	ctx := context.Background()
	bs := &Server{
		AttestationsPool: attestations.NewPool(),
	}

	atts := make([]*zondpb.Attestation, params.BeaconConfig().DefaultPageSize+1)
	for i := 0; i < len(atts); i++ {
		att := util.NewAttestation()
		att.Data.Slot = primitives.Slot(i)
		atts[i] = att
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))

	req := &zondpb.AttestationPoolRequest{}
	res, err := bs.AttestationPool(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, params.BeaconConfig().DefaultPageSize, len(res.Attestations), "Unexpected number of attestations")
	assert.Equal(t, params.BeaconConfig().DefaultPageSize+1, int(res.TotalSize), "Unexpected total size")
}

func TestServer_AttestationPool_Pagination_CustomPageSize(t *testing.T) {
	ctx := context.Background()
	bs := &Server{
		AttestationsPool: attestations.NewPool(),
	}

	numAtts := 100
	atts := make([]*zondpb.Attestation, numAtts)
	for i := 0; i < len(atts); i++ {
		att := util.NewAttestation()
		att.Data.Slot = primitives.Slot(i)
		atts[i] = att
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))
	tests := []struct {
		req *zondpb.AttestationPoolRequest
		res *zondpb.AttestationPoolResponse
	}{
		{
			req: &zondpb.AttestationPoolRequest{
				PageToken: strconv.Itoa(1),
				PageSize:  3,
			},
			res: &zondpb.AttestationPoolResponse{
				NextPageToken: "2",
				TotalSize:     int32(numAtts),
			},
		},
		{
			req: &zondpb.AttestationPoolRequest{
				PageToken: strconv.Itoa(3),
				PageSize:  30,
			},
			res: &zondpb.AttestationPoolResponse{
				NextPageToken: "",
				TotalSize:     int32(numAtts),
			},
		},
		{
			req: &zondpb.AttestationPoolRequest{
				PageToken: strconv.Itoa(0),
				PageSize:  int32(numAtts),
			},
			res: &zondpb.AttestationPoolResponse{
				NextPageToken: "",
				TotalSize:     int32(numAtts),
			},
		},
	}
	for _, tt := range tests {
		res, err := bs.AttestationPool(ctx, tt.req)
		require.NoError(t, err)
		assert.Equal(t, tt.res.TotalSize, res.TotalSize, "Unexpected total size")
		assert.Equal(t, tt.res.NextPageToken, res.NextPageToken, "Unexpected next page token")
	}
}
