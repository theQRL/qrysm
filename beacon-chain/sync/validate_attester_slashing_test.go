package sync

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/p2p"
	p2ptest "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	"github.com/theQRL/qrysm/beacon-chain/state"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/container/slice"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func setupValidAttesterSlashing(t *testing.T) (*qrysmpb.AttesterSlashing, state.BeaconState) {
	s, privKeys := util.DeterministicGenesisStateZond(t, 5)
	vals := s.Validators()
	for _, vv := range vals {
		vv.WithdrawableEpoch = primitives.Epoch(1 * params.BeaconConfig().SlotsPerEpoch)
	}
	require.NoError(t, s.SetValidators(vals))

	att1 := util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{
		Data: &qrysmpb.AttestationData{
			Source: &qrysmpb.Checkpoint{Epoch: 1},
		},
		AttestingIndices: []uint64{0, 1},
	})
	domain, err := signing.Domain(s.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, s.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att1.Data, domain)
	assert.NoError(t, err)
	lsig1, err := privKeys[0].Sign(hashTreeRoot[:])
	require.NoError(t, err)
	sig0 := lsig1.Marshal()
	lsig2, err := privKeys[1].Sign(hashTreeRoot[:])
	require.NoError(t, err)
	sig1 := lsig2.Marshal()
	att1.Signatures = [][]byte{sig0, sig1}

	att2 := util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{
		AttestingIndices: []uint64{0, 1},
	})
	hashTreeRoot, err = signing.ComputeSigningRoot(att2.Data, domain)
	assert.NoError(t, err)
	lsig3, err := privKeys[0].Sign(hashTreeRoot[:])
	require.NoError(t, err)
	sig0 = lsig3.Marshal()
	lsig4, err := privKeys[1].Sign(hashTreeRoot[:])
	require.NoError(t, err)
	sig1 = lsig4.Marshal()
	att2.Signatures = [][]byte{sig0, sig1}

	slashing := &qrysmpb.AttesterSlashing{
		Attestation_1: att1,
		Attestation_2: att2,
	}

	currentSlot := 2 * params.BeaconConfig().SlotsPerEpoch
	require.NoError(t, s.SetSlot(currentSlot))

	return slashing, s
}

func TestValidateAttesterSlashing_ValidSlashing(t *testing.T) {
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()

	slashing, s := setupValidAttesterSlashing(t)

	chain := &mock.ChainService{State: s, Genesis: time.Now()}
	r := &Service{
		cfg: &config{
			p2p:         p,
			chain:       chain,
			clock:       startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		seenAttesterSlashingCache: make(map[uint64]bool),
		subHandler:                newSubTopicHandler(),
	}

	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, slashing)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.AttesterSlashing]()]
	d, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, d)
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateAttesterSlashing(ctx, "foobar", msg)
	assert.NoError(t, err)
	valid := res == pubsub.ValidationAccept

	assert.Equal(t, true, valid, "Failed Validation")
	assert.NotNil(t, msg.ValidatorData, "Decoded message was not set on the message validator data")
}

func TestValidateAttesterSlashing_OutOfRangeIndex_Rejected(t *testing.T) {
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()

	slashing, s := setupValidAttesterSlashing(t)
	// Append an index one past the 5-validator registry to both attestations,
	// with a garbage signature in that position. Under the old behaviour the
	// slot was verified against an all-zero, universally forgeable key and the
	// message ended up Ignored (no peer penalty) at the validator lookup.
	outOfRange := uint64(s.NumValidators())
	for _, att := range []*qrysmpb.IndexedAttestation{slashing.Attestation_1, slashing.Attestation_2} {
		att.AttestingIndices = append(att.AttestingIndices, outOfRange)
		att.Signatures = append(att.Signatures, make([]byte, len(att.Signatures[0])))
	}

	chain := &mock.ChainService{State: s, Genesis: time.Now()}
	r := &Service{
		cfg: &config{
			p2p:         p,
			chain:       chain,
			clock:       startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		seenAttesterSlashingCache: make(map[uint64]bool),
		subHandler:                newSubTopicHandler(),
	}

	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, slashing)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.AttesterSlashing]()]
	d, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, d)
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateAttesterSlashing(ctx, "foobar", msg)
	assert.ErrorContains(t, "out of range", err)
	assert.Equal(t, pubsub.ValidationReject, res, "out-of-registry attesting index must be rejected, not ignored")
}

func TestValidateAttesterSlashing_ValidOldSlashing(t *testing.T) {
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()

	slashing, s := setupValidAttesterSlashing(t)
	vals := s.Validators()
	for _, v := range vals {
		v.Slashed = true
	}
	require.NoError(t, s.SetValidators(vals))
	chain := &mock.ChainService{State: s, Genesis: time.Now()}
	r := &Service{
		cfg: &config{
			p2p:         p,
			chain:       chain,
			clock:       startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		seenAttesterSlashingCache: make(map[uint64]bool),
		subHandler:                newSubTopicHandler(),
	}

	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, slashing)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.AttesterSlashing]()]
	d, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, d)
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateAttesterSlashing(ctx, "foobar", msg)
	assert.ErrorContains(t, "validators were previously slashed", err)
	valid := res == pubsub.ValidationIgnore

	assert.Equal(t, true, valid, "Incorrect Validation")
}

func TestValidateAttesterSlashing_ChecksEligibilityBeforeSignatures(t *testing.T) {
	p := p2ptest.NewTestP2P(t)
	t.Cleanup(func() { require.NoError(t, p.BHost.Close()) })
	slashing, beaconState := setupValidAttesterSlashing(t)

	for _, tt := range []struct {
		name                  string
		slashed               bool
		withdrawn             bool
		invalidAttestation    int
		outOfRangeAttestation int
		wantResult            pubsub.ValidationResult
		wantError             string
		wantPubkeyLookups     int
	}{
		{name: "valid", wantResult: pubsub.ValidationAccept, wantPubkeyLookups: 4},
		{name: "already slashed", slashed: true, wantResult: pubsub.ValidationIgnore, wantError: "validators were previously slashed"},
		{name: "already slashed with invalid signatures", slashed: true, invalidAttestation: 1, wantResult: pubsub.ValidationIgnore, wantError: "validators were previously slashed"},
		{name: "withdrawn with invalid signatures", withdrawn: true, invalidAttestation: 1, wantResult: pubsub.ValidationReject, wantError: "none of the validators are slashable"},
		{name: "invalid first attestation", invalidAttestation: 1, wantResult: pubsub.ValidationReject, wantError: "signature did not verify", wantPubkeyLookups: 2},
		{name: "invalid second attestation", invalidAttestation: 2, wantResult: pubsub.ValidationReject, wantError: "signature did not verify", wantPubkeyLookups: 4},
		{name: "already slashed with first attestation out of range", slashed: true, outOfRangeAttestation: 1, wantResult: pubsub.ValidationReject, wantError: "out of range"},
		{name: "already slashed with second attestation out of range", slashed: true, outOfRangeAttestation: 2, wantResult: pubsub.ValidationReject, wantError: "out of range"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			st := &pubkeyTrackingState{BeaconState: beaconState.Copy()}
			vals := st.Validators()
			for _, val := range vals {
				val.Slashed = tt.slashed
				if tt.withdrawn {
					val.WithdrawableEpoch = 0
				}
			}
			require.NoError(t, st.SetValidators(vals))
			request := &qrysmpb.AttesterSlashing{
				Attestation_1: qrysmpb.CopyIndexedAttestation(slashing.Attestation_1),
				Attestation_2: qrysmpb.CopyIndexedAttestation(slashing.Attestation_2),
			}
			atts := []*qrysmpb.IndexedAttestation{request.Attestation_1, request.Attestation_2}
			if tt.invalidAttestation != 0 {
				atts[tt.invalidAttestation-1].Signatures[0][0] ^= 1
			}
			if tt.outOfRangeAttestation != 0 {
				att := atts[tt.outOfRangeAttestation-1]
				att.AttestingIndices = append(att.AttestingIndices, uint64(st.NumValidators()))
				att.Signatures = append(att.Signatures, bytes.Clone(att.Signatures[0]))
			}

			s := &Service{cfg: &config{
				p2p:         p,
				chain:       &mock.ChainService{State: st},
				initialSync: &mockSync.Sync{},
			}}
			buf := new(bytes.Buffer)
			_, err := p.Encoding().EncodeGossip(buf, request)
			require.NoError(t, err)
			topic := fmt.Sprintf(p2p.AttesterSlashingSubnetTopicFormat, [4]byte{})
			msg := &pubsub.Message{Message: &pubsubpb.Message{Data: buf.Bytes(), Topic: &topic}}
			result, err := s.validateAttesterSlashing(t.Context(), "remote", msg)
			assert.Equal(t, tt.wantResult, result)
			if tt.wantError == "" {
				assert.NoError(t, err)
				assert.NotNil(t, msg.ValidatorData)
			} else {
				assert.ErrorContains(t, tt.wantError, err)
				assert.Equal(t, nil, msg.ValidatorData)
			}
			assert.Equal(t, tt.wantPubkeyLookups, st.pubkeyLookups, "ineligible slashings must not trigger signature work")
		})
	}
}

func TestValidateAttesterSlashing_InvalidSlashing_WithdrawableEpoch(t *testing.T) {
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()

	slashing, s := setupValidAttesterSlashing(t)
	// Set only one of the  validators as withdrawn
	vals := s.Validators()
	vals[1].WithdrawableEpoch = primitives.Epoch(1)

	require.NoError(t, s.SetValidators(vals))

	chain := &mock.ChainService{State: s, Genesis: time.Now()}
	r := &Service{
		cfg: &config{
			p2p:         p,
			chain:       chain,
			clock:       startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		seenAttesterSlashingCache: make(map[uint64]bool),
		subHandler:                newSubTopicHandler(),
	}

	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, slashing)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.AttesterSlashing]()]
	d, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, d)
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateAttesterSlashing(ctx, "foobar", msg)
	assert.NoError(t, err)
	valid := res == pubsub.ValidationAccept

	assert.Equal(t, true, valid, "Rejected Validation")

	// Set all validators as withdrawn.
	vals = s.Validators()
	for _, vv := range vals {
		vv.WithdrawableEpoch = primitives.Epoch(1)
	}

	require.NoError(t, s.SetValidators(vals))
	res, err = r.validateAttesterSlashing(ctx, "foobar", msg)
	assert.ErrorContains(t, "none of the validators are slashable", err)
	invalid := res == pubsub.ValidationReject

	assert.Equal(t, true, invalid, "Passed Validation")
}

func TestValidateAttesterSlashing_CanFilter(t *testing.T) {
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()

	chain := &mock.ChainService{Genesis: time.Now()}
	r := &Service{
		cfg: &config{
			p2p:         p,
			initialSync: &mockSync.Sync{IsSyncing: false},
			chain:       chain,
			clock:       startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
		},
		seenAttesterSlashingCache: make(map[uint64]bool),
		subHandler:                newSubTopicHandler(),
	}

	r.setAttesterSlashingIndicesSeen([]uint64{1, 2, 3, 4}, []uint64{3, 4, 5, 6})

	// The below attestations should be filtered hence bad signature is ok.
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.AttesterSlashing]()]
	d, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, d)
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, &qrysmpb.AttesterSlashing{
		Attestation_1: util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{
			AttestingIndices: []uint64{3},
		}),
		Attestation_2: util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{
			AttestingIndices: []uint64{3},
		}),
	})
	require.NoError(t, err)
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateAttesterSlashing(ctx, "foobar", msg)
	_ = err
	ignored := res == pubsub.ValidationIgnore
	assert.Equal(t, true, ignored)

	buf = new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, &qrysmpb.AttesterSlashing{
		Attestation_1: util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{
			AttestingIndices: []uint64{4, 3},
		}),
		Attestation_2: util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{
			AttestingIndices: []uint64{3, 4},
		}),
	})
	require.NoError(t, err)
	msg = &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err = r.validateAttesterSlashing(ctx, "foobar", msg)
	_ = err
	ignored = res == pubsub.ValidationIgnore
	assert.Equal(t, true, ignored)
}

func TestValidateAttesterSlashing_ContextTimeout(t *testing.T) {
	p := p2ptest.NewTestP2P(t)

	slashing, s := setupValidAttesterSlashing(t)
	slashing.Attestation_1.Data.Target.Epoch = 100000000

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	chain := &mock.ChainService{State: s}
	r := &Service{
		cfg: &config{
			p2p:         p,
			chain:       chain,
			clock:       startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		seenAttesterSlashingCache: make(map[uint64]bool),
	}

	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, slashing)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.AttesterSlashing]()]
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateAttesterSlashing(ctx, "foobar", msg)
	_ = err
	valid := res == pubsub.ValidationAccept
	assert.Equal(t, false, valid, "slashing from the far distant future should have timed out and returned false")
}

func TestValidateAttesterSlashing_Syncing(t *testing.T) {
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()

	slashing, s := setupValidAttesterSlashing(t)

	r := &Service{
		cfg: &config{
			p2p:         p,
			chain:       &mock.ChainService{State: s},
			initialSync: &mockSync.Sync{IsSyncing: true},
		},
	}

	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, slashing)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.AttesterSlashing]()]
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateAttesterSlashing(ctx, "foobar", msg)
	_ = err
	valid := res == pubsub.ValidationAccept
	assert.Equal(t, false, valid, "Passed validation")
}

func TestSeenAttesterSlashingIndices(t *testing.T) {
	tt := []struct {
		saveIndices1  []uint64
		saveIndices2  []uint64
		checkIndices1 []uint64
		checkIndices2 []uint64
		seen          bool
	}{
		{
			saveIndices1:  []uint64{0, 1, 2},
			saveIndices2:  []uint64{0},
			checkIndices1: []uint64{0, 1, 2},
			checkIndices2: []uint64{0},
			seen:          true,
		},
		{
			saveIndices1:  []uint64{100, 99, 98},
			saveIndices2:  []uint64{99, 98, 97},
			checkIndices1: []uint64{99, 98},
			checkIndices2: []uint64{99, 98},
			seen:          true,
		},
		{
			saveIndices1:  []uint64{100},
			saveIndices2:  []uint64{100},
			checkIndices1: []uint64{100, 101},
			checkIndices2: []uint64{100, 101},
			seen:          false,
		},
		{
			saveIndices1:  []uint64{100, 99, 98},
			saveIndices2:  []uint64{99, 98, 97},
			checkIndices1: []uint64{99, 98, 97},
			checkIndices2: []uint64{99, 98, 97},
			seen:          false,
		},
		{
			saveIndices1:  []uint64{100, 99, 98},
			saveIndices2:  []uint64{99, 98, 97},
			checkIndices1: []uint64{101, 100},
			checkIndices2: []uint64{101},
			seen:          false,
		},
	}
	for _, tc := range tt {
		r := &Service{
			seenAttesterSlashingCache: map[uint64]bool{},
		}
		r.setAttesterSlashingIndicesSeen(tc.saveIndices1, tc.saveIndices2)
		slashedVals := slice.IntersectionUint64(tc.checkIndices1, tc.checkIndices2)
		assert.Equal(t, tc.seen, r.hasSeenAttesterSlashingIndices(slashedVals))
	}
}
