package sync

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/theQRL/go-bitfield"
	mockChain "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	dbtest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	p2ptest "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/theQRL/qrysm/cache/lru"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestService_validateCommitteeIndexBeaconAttestation(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	db := dbtest.SetupDB(t)
	chain := &mockChain.ChainService{
		// 1 slot ago.
		Genesis:          time.Now().Add(time.Duration(-1*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second),
		ValidatorsRoot:   [32]byte{'A'},
		ValidAttestation: true,
		DB:               db,
		Optimistic:       true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Service{
		ctx: ctx,
		cfg: &config{
			initialSync:         &mockSync.Sync{IsSyncing: false},
			p2p:                 p,
			beaconDB:            db,
			chain:               chain,
			clock:               startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attestationNotifier: (&mockChain.ChainService{}).OperationNotifier(),
		},
		blkRootToPendingAtts:             make(map[[32]byte][]*qrysmpb.SignedAggregateAttestationAndProof),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	s.initCaches()
	go s.verifierRoutine()

	invalidRoot := [32]byte{'A', 'B', 'C', 'D'}
	s.setBadBlock(ctx, invalidRoot)

	digest, err := s.currentForkDigest()
	require.NoError(t, err)

	blk := util.NewBeaconBlockZond()
	blk.Block.Slot = 1
	util.SaveBlock(t, ctx, db, blk)

	validBlockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	chain.FinalizedCheckPoint = &qrysmpb.Checkpoint{
		Root:  validBlockRoot[:],
		Epoch: 0,
	}

	validators := uint64(256)
	savedState, keys := util.DeterministicGenesisStateZond(t, validators)
	require.NoError(t, savedState.SetSlot(1))
	require.NoError(t, db.SaveState(context.Background(), savedState, validBlockRoot))
	chain.State = savedState

	tests := []struct {
		name                      string
		msg                       *qrysmpb.Attestation
		topic                     string
		validAttestationSignature bool
		want                      bool
	}{
		{
			name: "valid attestation signature",
			msg: &qrysmpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target: &qrysmpb.Checkpoint{
						Epoch: 0,
						Root:  validBlockRoot[:],
					},
					Source: &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/consensus/%x/beacon_attestation_1", digest),
			validAttestationSignature: true,
			want:                      true,
		},
		{
			name: "valid attestation signature with nil topic",
			msg: &qrysmpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target: &qrysmpb.Checkpoint{
						Epoch: 0,
						Root:  validBlockRoot[:],
					},
					Source: &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     "",
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "bad target epoch",
			msg: &qrysmpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target: &qrysmpb.Checkpoint{
						Epoch: 10,
						Root:  validBlockRoot[:],
					},
					Source: &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/consensus/%x/beacon_attestation_1", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "already seen",
			msg: &qrysmpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/consensus/%x/beacon_attestation_1", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "invalid beacon block",
			msg: &qrysmpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: invalidRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/consensus/%x/beacon_attestation_1", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "committee index exceeds committee length",
			msg: &qrysmpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  4,
					Slot:            1,
					Target:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/consensus/%x/beacon_attestation_2", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "wrong committee index",
			msg: &qrysmpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  2,
					Slot:            1,
					Target:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/consensus/%x/beacon_attestation_2", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "already aggregated",
			msg: &qrysmpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b1011},
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  1,
					Slot:            1,
					Target:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/consensus/%x/beacon_attestation_1", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "missing block",
			msg: &qrysmpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: bytesutil.PadTo([]byte("missing"), fieldparams.RootLength),
					CommitteeIndex:  1,
					Slot:            1,
					Target:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/consensus/%x/beacon_attestation_1", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "invalid attestation",
			msg: &qrysmpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  1,
					Slot:            1,
					Target:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/consensus/%x/beacon_attestation_1", digest),
			validAttestationSignature: false,
			want:                      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helpers.ClearCache()
			chain.ValidAttestation = tt.validAttestationSignature
			if tt.validAttestationSignature {
				com, err := helpers.BeaconCommitteeFromState(context.Background(), savedState, tt.msg.Data.Slot, tt.msg.Data.CommitteeIndex)
				require.NoError(t, err)
				domain, err := signing.Domain(savedState.Fork(), tt.msg.Data.Target.Epoch, params.BeaconConfig().DomainBeaconAttester, savedState.GenesisValidatorsRoot())
				require.NoError(t, err)
				attRoot, err := signing.ComputeSigningRoot(tt.msg.Data, domain)
				require.NoError(t, err)
				for i := 0; ; i++ {
					if tt.msg.AggregationBits.BitAt(uint64(i)) {
						tt.msg.Signatures = [][]byte{keys[com[i]].Sign(attRoot[:]).Marshal()}
						break
					}
				}
			} else {
				tt.msg.Signatures = [][]byte{make([]byte, 4627)}
			}
			buf := new(bytes.Buffer)
			_, err := p.Encoding().EncodeGossip(buf, tt.msg)
			require.NoError(t, err)
			m := &pubsub.Message{
				Message: &pubsubpb.Message{
					Data:  buf.Bytes(),
					Topic: &tt.topic,
				},
			}
			if tt.topic == "" {
				m.Message.Topic = nil
			}

			res, err := s.validateCommitteeIndexBeaconAttestation(ctx, "" /*peerID*/, m)
			received := res == pubsub.ValidationAccept
			if received != tt.want {
				t.Fatalf("Did not received wanted validation. Got %v, wanted %v", !tt.want, tt.want)
			}
			if tt.want && err != nil {
				t.Errorf("Non nil error returned: %v", err)
			}
			if tt.want && m.ValidatorData == nil {
				t.Error("Expected validator data to be set")
			}
		})
	}
}

// Test_validateUnaggregatedAttTopic_CommitteeIndexBoundary pins the off-by-one
// fix: an attestation with CommitteeIndex == SlotCommitteeCount must be
// rejected. Valid indices are [0, count); without the fix the boundary case
// silently passes the count check.
func Test_validateUnaggregatedAttTopic_CommitteeIndexBoundary(t *testing.T) {
	ctx := context.Background()

	// 256 validators on mainnet config gives SlotCommitteeCount = 1, so
	// CommitteeIndex == 1 is the boundary value.
	validators := uint64(256)
	bs, _ := util.DeterministicGenesisStateZond(t, validators)
	require.NoError(t, bs.SetSlot(1))

	chain := &mockChain.ChainService{
		Genesis:        time.Now().Add(time.Duration(-1*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second),
		ValidatorsRoot: [32]byte{'A'},
	}
	s := &Service{cfg: &config{
		chain: chain,
		clock: startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
	}}

	digest, err := s.currentForkDigest()
	require.NoError(t, err)

	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Slot:           1,
			CommitteeIndex: 1,
		},
	}
	// Subnet for slot=1, count=1, committeeIndex=1: (1*1 + 1) % 64 = 2.
	// We pick the matching topic so the count check is what rejects the
	// attestation, not the subnet check.
	topic := fmt.Sprintf("/consensus/%x/beacon_attestation_2", digest)

	result, err := s.validateUnaggregatedAttTopic(ctx, att, bs, topic)
	require.Equal(t, pubsub.ValidationReject, result)
	require.ErrorContains(t, "committee index 1 >= 1", err)
}

func TestService_setSeenCommitteeIndicesSlot(t *testing.T) {
	s := NewService(context.Background(), WithP2P(p2ptest.NewTestP2P(t)))
	s.initCaches()

	// Empty cache
	b0 := []byte{9} // 1001
	require.Equal(t, false, s.hasSeenCommitteeIndicesSlot(0, 0, b0))

	// Cache some entries but same key
	s.setSeenCommitteeIndicesSlot(0, 0, b0)
	require.Equal(t, true, s.hasSeenCommitteeIndicesSlot(0, 0, b0))
	b1 := []byte{14} // 1110
	s.setSeenCommitteeIndicesSlot(0, 0, b1)
	require.Equal(t, true, s.hasSeenCommitteeIndicesSlot(0, 0, b0))
	require.Equal(t, true, s.hasSeenCommitteeIndicesSlot(0, 0, b1))

	// Cache some entries with diff keys
	s.setSeenCommitteeIndicesSlot(1, 2, b1)
	require.Equal(t, false, s.hasSeenCommitteeIndicesSlot(1, 0, b1))
	require.Equal(t, false, s.hasSeenCommitteeIndicesSlot(0, 2, b1))
	require.Equal(t, true, s.hasSeenCommitteeIndicesSlot(1, 2, b1))
}
