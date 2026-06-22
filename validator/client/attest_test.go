package client

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/async/event"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/config/features"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87/ml_dsa_87t"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	qrysmTime "github.com/theQRL/qrysm/time"
)

func TestRequestAttestation_ValidatorDutiesRequestFailure(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, _, validatorKey, finish := setup(t)
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{}}
	defer finish()

	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.SubmitAttestation(context.Background(), 30, pubKey)
	require.LogsContain(t, hook, "Could not fetch validator assignment")
}

func TestAttestToBlockHead_SubmitAttestation_EmptyCommittee(t *testing.T) {
	hook := logTest.NewGlobal()

	validator, _, validatorKey, finish := setup(t)
	defer finish()
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			CommitteeIndex: 0,
			Committee:      make([]primitives.ValidatorIndex, 0),
			ValidatorIndex: 0,
		}}}
	validator.SubmitAttestation(context.Background(), 0, pubKey)
	require.LogsContain(t, hook, "Empty committee")
}

func TestAttestToBlockHead_SubmitAttestation_RequestFailure(t *testing.T) {
	hook := logTest.NewGlobal()

	validator, m, validatorKey, finish := setup(t)
	defer finish()
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			CommitteeIndex: 5,
			Committee:      make([]primitives.ValidatorIndex, 111),
			ValidatorIndex: 0,
		}}}
	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.AttestationDataRequest{}),
	).Return(&qrysmpb.AttestationData{
		BeaconBlockRoot: make([]byte, fieldparams.RootLength),
		Target:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		Source:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
	}, nil)
	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch2
	).Times(2).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)
	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.Attestation{}),
	).Return(nil, errors.New("something went wrong"))

	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.SubmitAttestation(context.Background(), 30, pubKey)
	require.LogsContain(t, hook, "Could not submit attestation to beacon node")
}

func TestAttestToBlockHead_AttestsCorrectly(t *testing.T) {
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	hook := logTest.NewGlobal()
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			CommitteeIndex: 5,
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}

	beaconBlockRoot := bytesutil.ToBytes32([]byte("A"))
	targetRoot := bytesutil.ToBytes32([]byte("B"))
	sourceRoot := bytesutil.ToBytes32([]byte("C"))
	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.AttestationDataRequest{}),
	).Return(&qrysmpb.AttestationData{
		BeaconBlockRoot: beaconBlockRoot[:],
		Target:          &qrysmpb.Checkpoint{Root: targetRoot[:]},
		Source:          &qrysmpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
	}, nil)

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(2).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	var generatedAttestation *qrysmpb.Attestation
	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.Attestation{}),
	).Do(func(_ context.Context, att *qrysmpb.Attestation) {
		generatedAttestation = att
	}).Return(&qrysmpb.AttestResponse{}, nil)

	validator.SubmitAttestation(context.Background(), 30, pubKey)

	aggregationBitfield := bitfield.NewBitlist(uint64(len(committee)))
	aggregationBitfield.SetBitAt(4, true)
	expectedAttestation := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			BeaconBlockRoot: beaconBlockRoot[:],
			Target:          &qrysmpb.Checkpoint{Root: targetRoot[:]},
			Source:          &qrysmpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
		},
		AggregationBits: aggregationBitfield,
	}

	root, err := signing.ComputeSigningRoot(expectedAttestation.Data, make([]byte, 32))
	require.NoError(t, err)

	require.NotNil(t, generatedAttestation)
	require.DeepEqual(t, expectedAttestation.Data, generatedAttestation.Data)
	require.DeepEqual(t, expectedAttestation.AggregationBits, generatedAttestation.AggregationBits)
	require.Equal(t, 1, len(generatedAttestation.Signatures))
	require.Equal(t, field_params.MLDSA87SignatureLength, len(generatedAttestation.Signatures[0]))
	verifierPubKey, err := ml_dsa_87t.PublicKeyFromBytes(validatorKey.PublicKey().Marshal())
	require.NoError(t, err)
	valid, err := ml_dsa_87t.VerifySignature(generatedAttestation.Signatures[0], root, verifierPubKey)
	require.NoError(t, err)
	require.Equal(t, true, valid)
	require.LogsDoNotContain(t, hook, "Could not")
}

func TestAttestToBlockHead_BlocksDoubleAtt(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			CommitteeIndex: 5,
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	beaconBlockRoot := bytesutil.ToBytes32([]byte("A"))
	targetRoot := bytesutil.ToBytes32([]byte("B"))
	sourceRoot := bytesutil.ToBytes32([]byte("C"))
	beaconBlockRoot2 := bytesutil.ToBytes32([]byte("D"))

	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.AttestationDataRequest{}),
	).Return(&qrysmpb.AttestationData{
		BeaconBlockRoot: beaconBlockRoot[:],
		Target:          &qrysmpb.Checkpoint{Root: targetRoot[:], Epoch: 4},
		Source:          &qrysmpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
	}, nil)
	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.AttestationDataRequest{}),
	).Return(&qrysmpb.AttestationData{
		BeaconBlockRoot: beaconBlockRoot2[:],
		Target:          &qrysmpb.Checkpoint{Root: targetRoot[:], Epoch: 4},
		Source:          &qrysmpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
	}, nil)
	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(4).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.Attestation{}),
	).Return(&qrysmpb.AttestResponse{AttestationDataRoot: make([]byte, 32)}, nil /* error */)

	validator.SubmitAttestation(context.Background(), 30, pubKey)
	validator.SubmitAttestation(context.Background(), 30, pubKey)
	require.LogsContain(t, hook, "Failed attestation slashing protection")
}

func TestAttestToBlockHead_BlocksSurroundAtt(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			CommitteeIndex: 5,
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	beaconBlockRoot := bytesutil.ToBytes32([]byte("A"))
	targetRoot := bytesutil.ToBytes32([]byte("B"))
	sourceRoot := bytesutil.ToBytes32([]byte("C"))

	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.AttestationDataRequest{}),
	).Return(&qrysmpb.AttestationData{
		BeaconBlockRoot: beaconBlockRoot[:],
		Target:          &qrysmpb.Checkpoint{Root: targetRoot[:], Epoch: 2},
		Source:          &qrysmpb.Checkpoint{Root: sourceRoot[:], Epoch: 1},
	}, nil)
	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.AttestationDataRequest{}),
	).Return(&qrysmpb.AttestationData{
		BeaconBlockRoot: beaconBlockRoot[:],
		Target:          &qrysmpb.Checkpoint{Root: targetRoot[:], Epoch: 3},
		Source:          &qrysmpb.Checkpoint{Root: sourceRoot[:], Epoch: 0},
	}, nil)

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(4).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.Attestation{}),
	).Return(&qrysmpb.AttestResponse{}, nil /* error */)

	validator.SubmitAttestation(context.Background(), 30, pubKey)
	validator.SubmitAttestation(context.Background(), 30, pubKey)
	require.LogsContain(t, hook, "Failed attestation slashing protection")
}

func TestAttestToBlockHead_BlocksSurroundedAtt(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	validatorIndex := primitives.ValidatorIndex(7)
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			CommitteeIndex: 5,
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	beaconBlockRoot := bytesutil.ToBytes32([]byte("A"))
	targetRoot := bytesutil.ToBytes32([]byte("B"))
	sourceRoot := bytesutil.ToBytes32([]byte("C"))

	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.AttestationDataRequest{}),
	).Return(&qrysmpb.AttestationData{
		BeaconBlockRoot: beaconBlockRoot[:],
		Target:          &qrysmpb.Checkpoint{Root: targetRoot[:], Epoch: 3},
		Source:          &qrysmpb.Checkpoint{Root: sourceRoot[:], Epoch: 0},
	}, nil)

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(4).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.Attestation{}),
	).Return(&qrysmpb.AttestResponse{}, nil /* error */)

	validator.SubmitAttestation(context.Background(), 30, pubKey)
	require.LogsDoNotContain(t, hook, failedAttLocalProtectionErr)

	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.AttestationDataRequest{}),
	).Return(&qrysmpb.AttestationData{
		BeaconBlockRoot: bytesutil.PadTo([]byte("A"), 32),
		Target:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte("B"), 32), Epoch: 2},
		Source:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte("C"), 32), Epoch: 1},
	}, nil)

	validator.SubmitAttestation(context.Background(), 30, pubKey)
	require.LogsContain(t, hook, "Failed attestation slashing protection")
}

func TestAttestToBlockHead_DoesNotAttestBeforeDelay(t *testing.T) {
	validator, m, validatorKey, finish := setup(t)
	defer finish()

	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.genesisTime = uint64(qrysmTime.Now().Unix())
	m.validatorClient.EXPECT().GetDuties(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.DutiesRequest{}),
	).Times(0)

	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.AttestationDataRequest{}),
	).Times(0)

	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.Attestation{}),
	).Return(&qrysmpb.AttestResponse{}, nil /* error */).Times(0)

	timer := time.NewTimer(1 * time.Second)
	go validator.SubmitAttestation(context.Background(), 0, pubKey)
	<-timer.C
}

func TestAttestToBlockHead_DoesAttestAfterDelay(t *testing.T) {
	cfg := params.BeaconConfig().Copy()
	cfg.SecondsPerSlot = 10
	params.OverrideBeaconConfig(cfg)

	validator, m, validatorKey, finish := setup(t)
	defer finish()

	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()

	validator.genesisTime = uint64(qrysmTime.Now().Unix())
	validatorIndex := primitives.ValidatorIndex(5)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			CommitteeIndex: 5,
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		}}}

	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.AttestationDataRequest{}),
	).Return(&qrysmpb.AttestationData{
		BeaconBlockRoot: bytesutil.PadTo([]byte("A"), 32),
		Target:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte("B"), 32)},
		Source:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte("C"), 32), Epoch: 3},
	}, nil).Do(func(arg0, arg1 any) {
		wg.Done()
	})

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(2).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.Any(),
	).Return(&qrysmpb.AttestResponse{}, nil).Times(1)

	validator.SubmitAttestation(context.Background(), 0, pubKey)
}

func TestAttestToBlockHead_CorrectBitfieldLength(t *testing.T) {
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	validatorIndex := primitives.ValidatorIndex(2)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			CommitteeIndex: 5,
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		}}}
	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.AttestationDataRequest{}),
	).Return(&qrysmpb.AttestationData{
		Target:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte("B"), 32)},
		Source:          &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte("C"), 32), Epoch: 3},
		BeaconBlockRoot: make([]byte, fieldparams.RootLength),
	}, nil)

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(2).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	var generatedAttestation *qrysmpb.Attestation
	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.Attestation{}),
	).Do(func(_ context.Context, att *qrysmpb.Attestation) {
		generatedAttestation = att
	}).Return(&qrysmpb.AttestResponse{}, nil /* error */)

	validator.SubmitAttestation(context.Background(), 30, pubKey)

	assert.Equal(t, 2, len(generatedAttestation.AggregationBits))
}

func TestSignAttestation(t *testing.T) {
	validator, m, _, finish := setup(t)
	defer finish()
	wantedFork := &qrysmpb.Fork{
		PreviousVersion: []byte{'a', 'b', 'c', 'd'},
		CurrentVersion:  []byte{'d', 'e', 'f', 'f'},
		Epoch:           0,
	}
	genesisValidatorsRoot := [32]byte{0x01, 0x02}
	attesterDomain, err := signing.Domain(wantedFork, 0, params.BeaconConfig().DomainBeaconAttester, genesisValidatorsRoot[:])
	require.NoError(t, err)
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&qrysmpb.DomainResponse{SignatureDomain: attesterDomain}, nil)
	ctx := context.Background()
	att := util.NewAttestation()
	att.Data.Source.Epoch = 100
	att.Data.Target.Epoch = 200
	att.Data.Slot = 999
	att.Data.BeaconBlockRoot = bytesutil.PadTo([]byte("blockRoot"), 32)

	pk := testKeyFromBytes(t, []byte{1})
	validator.keyManager = newMockKeymanager(t, pk)
	sig, sr, err := validator.signAtt(ctx, pk.pub, att.Data, att.Data.Slot)
	require.NoError(t, err, "%x,%x,%v", sig, sr, err)
	// proposer domain
	require.DeepEqual(t, "02bbdb88056d6cbafd6e94575540"+
		"e74b8cf2c0f2c1b79b8e17e7b21ed1694305", hex.EncodeToString(sr[:]))
}

func TestServer_WaitToSlotOneThird_CanWait(t *testing.T) {
	cfg := params.BeaconConfig().Copy()
	cfg.SecondsPerSlot = 10
	params.OverrideBeaconConfig(cfg)

	currentTime := uint64(time.Now().Unix())
	currentSlot := primitives.Slot(4)
	genesisTime := currentTime - uint64(currentSlot.Mul(params.BeaconConfig().SecondsPerSlot))

	v := &validator{
		genesisTime: genesisTime,
		blockFeed:   new(event.Feed),
	}

	timeToSleep := params.BeaconConfig().SecondsPerSlot / 3
	oneThird := currentTime + timeToSleep
	v.waitOneThirdOrValidBlock(context.Background(), currentSlot)

	if oneThird != uint64(time.Now().Unix()) {
		t.Errorf("Wanted %d time for slot one third but got %d", oneThird, currentTime)
	}
}

func TestServer_WaitToSlotOneThird_SameReqSlot(t *testing.T) {
	currentTime := uint64(time.Now().Unix())
	currentSlot := primitives.Slot(4)
	genesisTime := currentTime - uint64(currentSlot.Mul(params.BeaconConfig().SecondsPerSlot))

	v := &validator{
		genesisTime:      genesisTime,
		blockFeed:        new(event.Feed),
		highestValidSlot: currentSlot,
	}

	v.waitOneThirdOrValidBlock(context.Background(), currentSlot)

	if currentTime != uint64(time.Now().Unix()) {
		t.Errorf("Wanted %d time for slot one third but got %d", uint64(time.Now().Unix()), currentTime)
	}
}

func TestServer_WaitToSlotOneThird_ReceiveBlockSlot(t *testing.T) {
	cfg := params.BeaconConfig().Copy()
	cfg.SecondsPerSlot = 10
	params.OverrideBeaconConfig(cfg)

	resetCfg := features.InitWithReset(&features.Flags{AttestTimely: true})
	defer resetCfg()

	currentTime := uint64(time.Now().Unix())
	currentSlot := primitives.Slot(4)
	genesisTime := currentTime - uint64(currentSlot.Mul(params.BeaconConfig().SecondsPerSlot))

	v := &validator{
		genesisTime: genesisTime,
		blockFeed:   new(event.Feed),
	}

	wg := &sync.WaitGroup{}
	wg.Go(func() {
		time.Sleep(100 * time.Millisecond)
		wsb, err := blocks.NewSignedBeaconBlock(
			&qrysmpb.SignedBeaconBlockZond{
				Block: &qrysmpb.BeaconBlockZond{Slot: currentSlot, Body: &qrysmpb.BeaconBlockBodyZond{}},
			})
		require.NoError(t, err)
		v.blockFeed.Send(wsb)
	})

	v.waitOneThirdOrValidBlock(context.Background(), currentSlot)

	if currentTime != uint64(time.Now().Unix()) {
		t.Errorf("Wanted %d time for slot one third but got %d", uint64(time.Now().Unix()), currentTime)
	}
}
