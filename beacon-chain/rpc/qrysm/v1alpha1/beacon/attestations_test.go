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
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
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

	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot: 0,
	})
	require.NoError(t, err)
	bs := &Server{
		BeaconDB: db,
		HeadFetcher: &chainMock.ChainService{
			State: st,
		},
	}
	wanted := &qrysmpb.ListAttestationsResponse{
		Attestations:  make([]*qrysmpb.Attestation, 0),
		TotalSize:     int32(0),
		NextPageToken: strconv.Itoa(0),
	}
	res, err := bs.ListAttestations(ctx, &qrysmpb.ListAttestationsRequest{
		QueryFilter: &qrysmpb.ListAttestationsRequest_GenesisEpoch{GenesisEpoch: true},
	})
	require.NoError(t, err)
	if !proto.Equal(wanted, res) {
		t.Errorf("Wanted %v, received %v", wanted, res)
	}
}

func TestServer_ListAttestations_Genesis(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot: 0,
	})
	require.NoError(t, err)
	bs := &Server{
		BeaconDB: db,
		HeadFetcher: &chainMock.ChainService{
			State: st,
		},
	}

	att := util.HydrateAttestation(&qrysmpb.Attestation{
		AggregationBits: bitfield.NewBitlist(0),
		Data: &qrysmpb.AttestationData{
			Slot:           2,
			CommitteeIndex: 1,
		},
	})

	parentRoot := [32]byte{1, 2, 3}
	signedBlock := util.NewBeaconBlockZond()
	signedBlock.Block.ParentRoot = bytesutil.PadTo(parentRoot[:], 32)
	signedBlock.Block.Body.Attestations = []*qrysmpb.Attestation{att}
	root, err := signedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, db, signedBlock)
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))
	wanted := &qrysmpb.ListAttestationsResponse{
		Attestations:  []*qrysmpb.Attestation{att},
		NextPageToken: "",
		TotalSize:     1,
	}

	res, err := bs.ListAttestations(ctx, &qrysmpb.ListAttestationsRequest{
		QueryFilter: &qrysmpb.ListAttestationsRequest_GenesisEpoch{
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
	atts := make([]*qrysmpb.Attestation, 0, count)
	for i := range count {
		blockExample := util.NewBeaconBlockZond()
		blockExample.Block.Body.Attestations = []*qrysmpb.Attestation{
			{
				Signatures: [][]byte{make([]byte, field_params.MLDSA87SignatureLength)},
				Data: &qrysmpb.AttestationData{
					Target:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte("root"), 32)},
					Source:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte("root"), 32)},
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

	received, err := bs.ListAttestations(ctx, &qrysmpb.ListAttestationsRequest{
		QueryFilter: &qrysmpb.ListAttestationsRequest_GenesisEpoch{
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

	unwrappedBlocks := []*qrysmpb.SignedBeaconBlockZond{
		util.HydrateSignedBeaconBlockZond(
			&qrysmpb.SignedBeaconBlockZond{
				Block: &qrysmpb.BeaconBlockZond{
					Slot: 4,
					Body: &qrysmpb.BeaconBlockBodyZond{
						Attestations: []*qrysmpb.Attestation{
							{
								Data: &qrysmpb.AttestationData{
									BeaconBlockRoot: someRoot[:],
									Source: &qrysmpb.Checkpoint{
										Root:  sourceRoot[:],
										Epoch: sourceEpoch,
									},
									Target: &qrysmpb.Checkpoint{
										Root:  targetRoot[:],
										Epoch: targetEpoch,
									},
									Slot: 3,
								},
								AggregationBits: bitfield.Bitlist{0b11},
								Signatures:      [][]byte{bytesutil.PadTo([]byte("sig"), fieldparams.MLDSA87SignatureLength)},
							},
						},
					},
				},
			}),
		util.HydrateSignedBeaconBlockZond(&qrysmpb.SignedBeaconBlockZond{
			Block: &qrysmpb.BeaconBlockZond{
				Slot: 5 + params.BeaconConfig().SlotsPerEpoch,
				Body: &qrysmpb.BeaconBlockBodyZond{
					Attestations: []*qrysmpb.Attestation{
						{
							Data: &qrysmpb.AttestationData{
								BeaconBlockRoot: someRoot[:],
								Source: &qrysmpb.Checkpoint{
									Root:  sourceRoot[:],
									Epoch: sourceEpoch,
								},
								Target: &qrysmpb.Checkpoint{
									Root:  targetRoot[:],
									Epoch: targetEpoch,
								},
								Slot: 4 + params.BeaconConfig().SlotsPerEpoch,
							},
							AggregationBits: bitfield.Bitlist{0b11},
							Signatures:      [][]byte{bytesutil.PadTo([]byte("sig"), fieldparams.MLDSA87SignatureLength)},
						},
					},
				},
			},
		}),
		util.HydrateSignedBeaconBlockZond(
			&qrysmpb.SignedBeaconBlockZond{
				Block: &qrysmpb.BeaconBlockZond{
					Slot: 5,
					Body: &qrysmpb.BeaconBlockBodyZond{
						Attestations: []*qrysmpb.Attestation{
							{
								Data: &qrysmpb.AttestationData{
									BeaconBlockRoot: someRoot[:],
									Source: &qrysmpb.Checkpoint{
										Root:  sourceRoot[:],
										Epoch: sourceEpoch,
									},
									Target: &qrysmpb.Checkpoint{
										Root:  targetRoot[:],
										Epoch: targetEpoch,
									},
									Slot: 4,
								},
								AggregationBits: bitfield.Bitlist{0b11},
								Signatures:      [][]byte{bytesutil.PadTo([]byte("sig"), fieldparams.MLDSA87SignatureLength)},
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

	received, err := bs.ListAttestations(ctx, &qrysmpb.ListAttestationsRequest{
		QueryFilter: &qrysmpb.ListAttestationsRequest_Epoch{Epoch: 1},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, len(received.Attestations))
	received, err = bs.ListAttestations(ctx, &qrysmpb.ListAttestationsRequest{
		QueryFilter: &qrysmpb.ListAttestationsRequest_GenesisEpoch{GenesisEpoch: true},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, len(received.Attestations))
}

func TestServer_ListAttestations_Pagination_CustomPageParameters(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	count := params.BeaconConfig().SlotsPerEpoch * 4
	atts := make([]*qrysmpb.Attestation, 0, count)
	for i := primitives.Slot(0); i < params.BeaconConfig().SlotsPerEpoch; i++ {
		for s := range primitives.CommitteeIndex(4) {
			blockExample := util.NewBeaconBlockZond()
			blockExample.Block.Slot = i
			blockExample.Block.Body.Attestations = []*qrysmpb.Attestation{
				util.HydrateAttestation(&qrysmpb.Attestation{
					Data: &qrysmpb.AttestationData{
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
		req  *qrysmpb.ListAttestationsRequest
		res  *qrysmpb.ListAttestationsResponse
	}{
		{
			name: "1st of 3 pages",
			req: &qrysmpb.ListAttestationsRequest{
				QueryFilter: &qrysmpb.ListAttestationsRequest_GenesisEpoch{
					GenesisEpoch: true,
				},
				PageToken: strconv.Itoa(1),
				PageSize:  3,
			},
			res: &qrysmpb.ListAttestationsResponse{
				Attestations: []*qrysmpb.Attestation{
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
			req: &qrysmpb.ListAttestationsRequest{
				QueryFilter: &qrysmpb.ListAttestationsRequest_GenesisEpoch{
					GenesisEpoch: true,
				},
				PageToken: strconv.Itoa(10),
				PageSize:  1,
			},
			res: &qrysmpb.ListAttestationsResponse{
				Attestations: []*qrysmpb.Attestation{
					atts[10],
				},
				NextPageToken: strconv.Itoa(11),
				TotalSize:     int32(count),
			},
		},
		{
			name: "2 of size 8",
			req: &qrysmpb.ListAttestationsRequest{
				QueryFilter: &qrysmpb.ListAttestationsRequest_GenesisEpoch{
					GenesisEpoch: true,
				},
				PageToken: strconv.Itoa(2),
				PageSize:  8,
			},
			res: &qrysmpb.ListAttestationsResponse{
				Attestations: []*qrysmpb.Attestation{
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
	util.NewBeaconBlockZond()
	count := primitives.Slot(1)
	atts := make([]*qrysmpb.Attestation, 0, count)
	for i := range count {
		blockExample := util.HydrateSignedBeaconBlockZond(&qrysmpb.SignedBeaconBlockZond{
			Block: &qrysmpb.BeaconBlockZond{
				Body: &qrysmpb.BeaconBlockBodyZond{
					Attestations: []*qrysmpb.Attestation{
						{
							Data: &qrysmpb.AttestationData{
								BeaconBlockRoot: bytesutil.PadTo([]byte("root"), fieldparams.RootLength),
								Source:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
								Target:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
								Slot:            i,
							},
							AggregationBits: bitfield.Bitlist{0b11},
							Signatures:      [][]byte{make([]byte, fieldparams.MLDSA87SignatureLength)},
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

	req := &qrysmpb.ListAttestationsRequest{
		QueryFilter: &qrysmpb.ListAttestationsRequest_Epoch{
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
	req := &qrysmpb.ListAttestationsRequest{PageToken: strconv.Itoa(0), PageSize: exceedsMax}
	_, err := bs.ListAttestations(ctx, req)
	assert.ErrorContains(t, wanted, err)
}

func TestServer_ListAttestations_Pagination_DefaultPageSize(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	count := primitives.Slot(params.BeaconConfig().DefaultPageSize)
	atts := make([]*qrysmpb.Attestation, 0, count)
	for i := range count {
		blockExample := util.NewBeaconBlockZond()
		blockExample.Block.Body.Attestations = []*qrysmpb.Attestation{
			{
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: bytesutil.PadTo([]byte("root"), 32),
					Target:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte("root"), 32)},
					Source:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte("root"), 32)},
					Slot:            i,
				},
				Signatures:      [][]byte{bytesutil.PadTo([]byte("root"), fieldparams.MLDSA87SignatureLength)},
				AggregationBits: bitfield.Bitlist{0b11},
			},
		}
		util.SaveBlock(t, ctx, db, blockExample)
		atts = append(atts, blockExample.Block.Body.Attestations...)
	}

	bs := &Server{
		BeaconDB: db,
	}

	req := &qrysmpb.ListAttestationsRequest{
		QueryFilter: &qrysmpb.ListAttestationsRequest_GenesisEpoch{
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
	atts := make([]*qrysmpb.Attestation, count)
	targetRoot1 := bytesutil.ToBytes32([]byte("root1"))
	targetRoot2 := bytesutil.ToBytes32([]byte("root2"))

	for i := range count {
		var targetRoot [32]byte
		if i%2 == 0 {
			targetRoot = targetRoot1
		} else {
			targetRoot = targetRoot2
		}
		atts[i] = &qrysmpb.Attestation{
			Data: &qrysmpb.AttestationData{
				Target: &qrysmpb.Checkpoint{
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
	atts := make([]*qrysmpb.Attestation, 0, count)
	atts2 := make([]*qrysmpb.Attestation, 0, count)

	for i := range count {
		var targetRoot [32]byte
		if i%2 == 0 {
			targetRoot = targetRoot1
		} else {
			targetRoot = targetRoot2
		}
		blockExample := util.NewBeaconBlockZond()
		blockExample.Block.Body.Attestations = []*qrysmpb.Attestation{
			{
				Signatures: [][]byte{},
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
					Target: &qrysmpb.Checkpoint{
						Root: targetRoot[:],
					},
					Source: &qrysmpb.Checkpoint{
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
	state, _ := util.DeterministicGenesisStateZond(t, numValidators)

	// Next up we convert the test attestations to indexed form:
	indexedAtts := make([]*qrysmpb.IndexedAttestation, len(atts)+len(atts2))
	for i := range atts {
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
	err := db.SaveStateSummary(ctx, &qrysmpb.StateSummary{
		Root: targetRoot1[:],
		Slot: 1,
	})
	require.NoError(t, err)

	err = db.SaveStateSummary(ctx, &qrysmpb.StateSummary{
		Root: targetRoot2[:],
		Slot: 2,
	})
	require.NoError(t, err)

	require.NoError(t, db.SaveState(ctx, state, bytesutil.ToBytes32(targetRoot1[:])))
	require.NoError(t, state.SetSlot(state.Slot()+1))
	require.NoError(t, db.SaveState(ctx, state, bytesutil.ToBytes32(targetRoot2[:])))
	res, err := bs.ListIndexedAttestations(ctx, &qrysmpb.ListIndexedAttestationsRequest{
		QueryFilter: &qrysmpb.ListIndexedAttestationsRequest_GenesisEpoch{
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
	atts := make([]*qrysmpb.Attestation, 0, count)
	epoch := primitives.Epoch(50)
	startSlot, err := slots.EpochStart(epoch)
	require.NoError(t, err)

	for i := startSlot; i < count; i++ {
		blockExample := &qrysmpb.SignedBeaconBlockZond{
			Block: &qrysmpb.BeaconBlockZond{
				Body: &qrysmpb.BeaconBlockBodyZond{
					Attestations: []*qrysmpb.Attestation{
						{
							Data: &qrysmpb.AttestationData{
								BeaconBlockRoot: blockRoot[:],
								Slot:            i,
								CommitteeIndex:  0,
								Target: &qrysmpb.Checkpoint{
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
	state, _ := util.DeterministicGenesisStateZond(t, numValidators)

	randaoMixes := make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector)
	for i := range randaoMixes {
		randaoMixes[i] = make([]byte, fieldparams.RootLength)
	}
	require.NoError(t, state.SetRandaoMixes(randaoMixes))
	require.NoError(t, state.SetSlot(startSlot))

	// Next up we convert the test attestations to indexed form:
	indexedAtts := make([]*qrysmpb.IndexedAttestation, len(atts))
	for i := range atts {
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
	err = db.SaveStateSummary(ctx, &qrysmpb.StateSummary{
		Root: blockRoot[:],
		Slot: params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epoch)),
	})
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, state, bytesutil.ToBytes32([]byte("root"))))
	res, err := bs.ListIndexedAttestations(ctx, &qrysmpb.ListIndexedAttestationsRequest{
		QueryFilter: &qrysmpb.ListIndexedAttestationsRequest_Epoch{
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
	req := &qrysmpb.AttestationPoolRequest{PageToken: strconv.Itoa(0), PageSize: exceedsMax}
	_, err := bs.AttestationPool(ctx, req)
	assert.ErrorContains(t, wanted, err)
}

func TestServer_AttestationPool_Pagination_OutOfRange(t *testing.T) {
	ctx := context.Background()
	bs := &Server{
		AttestationsPool: attestations.NewPool(),
	}

	atts := []*qrysmpb.Attestation{
		{
			Data: &qrysmpb.AttestationData{
				Slot:            1,
				BeaconBlockRoot: bytesutil.PadTo([]byte{1}, 32),
				Source:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte{1}, 32)},
				Target:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte{1}, 32)},
			},
			AggregationBits: bitfield.Bitlist{0b1101},
			Signatures:      [][]byte{bytesutil.PadTo([]byte{1}, fieldparams.MLDSA87SignatureLength), bytesutil.PadTo([]byte{1}, fieldparams.MLDSA87SignatureLength)},
		},
		{
			Data: &qrysmpb.AttestationData{
				Slot:            2,
				BeaconBlockRoot: bytesutil.PadTo([]byte{2}, 32),
				Source:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte{2}, 32)},
				Target:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte{2}, 32)},
			},
			AggregationBits: bitfield.Bitlist{0b1101},
			Signatures:      [][]byte{bytesutil.PadTo([]byte{2}, fieldparams.MLDSA87SignatureLength), bytesutil.PadTo([]byte{2}, fieldparams.MLDSA87SignatureLength)},
		},
		{
			Data: &qrysmpb.AttestationData{
				Slot:            3,
				BeaconBlockRoot: bytesutil.PadTo([]byte{3}, 32),
				Source:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte{3}, 32)},
				Target:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte{3}, 32)},
			},
			AggregationBits: bitfield.Bitlist{0b1101},
			Signatures:      [][]byte{bytesutil.PadTo([]byte{3}, fieldparams.MLDSA87SignatureLength), bytesutil.PadTo([]byte{3}, fieldparams.MLDSA87SignatureLength)},
		},
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))

	req := &qrysmpb.AttestationPoolRequest{
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

	atts := make([]*qrysmpb.Attestation, params.BeaconConfig().DefaultPageSize+1)
	for i := range atts {
		att := util.NewAttestation()
		att.Data.Slot = primitives.Slot(i)
		atts[i] = att
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))

	req := &qrysmpb.AttestationPoolRequest{}
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
	atts := make([]*qrysmpb.Attestation, numAtts)
	for i := range atts {
		att := util.NewAttestation()
		att.Data.Slot = primitives.Slot(i)
		atts[i] = att
	}
	require.NoError(t, bs.AttestationsPool.SaveAggregatedAttestations(atts))
	tests := []struct {
		req *qrysmpb.AttestationPoolRequest
		res *qrysmpb.AttestationPoolResponse
	}{
		{
			req: &qrysmpb.AttestationPoolRequest{
				PageToken: strconv.Itoa(1),
				PageSize:  3,
			},
			res: &qrysmpb.AttestationPoolResponse{
				NextPageToken: "2",
				TotalSize:     int32(numAtts),
			},
		},
		{
			req: &qrysmpb.AttestationPoolRequest{
				PageToken: strconv.Itoa(3),
				PageSize:  30,
			},
			res: &qrysmpb.AttestationPoolResponse{
				NextPageToken: "",
				TotalSize:     int32(numAtts),
			},
		},
		{
			req: &qrysmpb.AttestationPoolRequest{
				PageToken: strconv.Itoa(0),
				PageSize:  int32(numAtts),
			},
			res: &qrysmpb.AttestationPoolResponse{
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
