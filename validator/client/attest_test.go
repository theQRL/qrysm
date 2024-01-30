package client

import (
	"context"
	"encoding/hex"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/async/event"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/v4/config/features"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	qrysmTime "github.com/theQRL/qrysm/v4/time"
	"gopkg.in/d4l3k/messagediff.v1"
)

func TestRequestAttestation_ValidatorDutiesRequestFailure(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, _, validatorKey, finish := setup(t)
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{}}
	defer finish()

	var pubKey [dilithium.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.SubmitAttestation(context.Background(), 30, pubKey)
	require.LogsContain(t, hook, "Could not fetch validator assignment")
}

func TestAttestToBlockHead_SubmitAttestation_EmptyCommittee(t *testing.T) {
	hook := logTest.NewGlobal()

	validator, _, validatorKey, finish := setup(t)
	defer finish()
	var pubKey [dilithium.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
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
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			CommitteeIndex: 5,
			Committee:      make([]primitives.ValidatorIndex, 111),
			ValidatorIndex: 0,
		}}}
	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.AttestationDataRequest{}),
	).Return(&zondpb.AttestationData{
		BeaconBlockRoot: make([]byte, fieldparams.RootLength),
		Target:          &zondpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		Source:          &zondpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
	}, nil)
	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch2
	).Times(2).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)
	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.Attestation{}),
	).Return(nil, errors.New("something went wrong"))

	var pubKey [dilithium.CryptoPublicKeyBytes]byte
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
	var pubKey [dilithium.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
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
		gomock.AssignableToTypeOf(&zondpb.AttestationDataRequest{}),
	).Return(&zondpb.AttestationData{
		BeaconBlockRoot: beaconBlockRoot[:],
		Target:          &zondpb.Checkpoint{Root: targetRoot[:]},
		Source:          &zondpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
	}, nil)

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(2).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	var generatedAttestation *zondpb.Attestation
	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.Attestation{}),
	).Do(func(_ context.Context, att *zondpb.Attestation) {
		generatedAttestation = att
	}).Return(&zondpb.AttestResponse{}, nil)

	validator.SubmitAttestation(context.Background(), 30, pubKey)

	aggregationBitfield := bitfield.NewBitlist(uint64(len(committee)))
	aggregationBitfield.SetBitAt(4, true)
	expectedAttestation := &zondpb.Attestation{
		Data: &zondpb.AttestationData{
			BeaconBlockRoot: beaconBlockRoot[:],
			Target:          &zondpb.Checkpoint{Root: targetRoot[:]},
			Source:          &zondpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
		},
		AggregationBits: aggregationBitfield,
		Signature:       make([]byte, 4595),
		// TODO(rgeraldes24): revisit once we review the proto definitions again
		// attest_test.go:166: modified: .SignatureValidatorIndex = []uint64{0x7}
		SignatureValidatorIndex: []uint64{0x7},
	}

	root, err := signing.ComputeSigningRoot(expectedAttestation.Data, make([]byte, 32))
	require.NoError(t, err)

	sig, err := validator.keyManager.Sign(context.Background(), &validatorpb.SignRequest{
		PublicKey:   validatorKey.PublicKey().Marshal(),
		SigningRoot: root[:],
	})
	require.NoError(t, err)
	expectedAttestation.Signature = sig.Marshal()
	if !reflect.DeepEqual(generatedAttestation, expectedAttestation) {
		t.Errorf("Incorrectly attested head, wanted %v, received %v", expectedAttestation, generatedAttestation)
		diff, _ := messagediff.PrettyDiff(expectedAttestation, generatedAttestation)
		t.Log(diff)
	}
	require.LogsDoNotContain(t, hook, "Could not")
}

func TestAttestToBlockHead_BlocksDoubleAtt(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	var pubKey [dilithium.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
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
		gomock.AssignableToTypeOf(&zondpb.AttestationDataRequest{}),
	).Return(&zondpb.AttestationData{
		BeaconBlockRoot: beaconBlockRoot[:],
		Target:          &zondpb.Checkpoint{Root: targetRoot[:], Epoch: 4},
		Source:          &zondpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
	}, nil)
	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.AttestationDataRequest{}),
	).Return(&zondpb.AttestationData{
		BeaconBlockRoot: beaconBlockRoot2[:],
		Target:          &zondpb.Checkpoint{Root: targetRoot[:], Epoch: 4},
		Source:          &zondpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
	}, nil)
	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(4).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.Attestation{}),
	).Return(&zondpb.AttestResponse{AttestationDataRoot: make([]byte, 32)}, nil /* error */)

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
	var pubKey [dilithium.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
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
		gomock.AssignableToTypeOf(&zondpb.AttestationDataRequest{}),
	).Return(&zondpb.AttestationData{
		BeaconBlockRoot: beaconBlockRoot[:],
		Target:          &zondpb.Checkpoint{Root: targetRoot[:], Epoch: 2},
		Source:          &zondpb.Checkpoint{Root: sourceRoot[:], Epoch: 1},
	}, nil)
	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.AttestationDataRequest{}),
	).Return(&zondpb.AttestationData{
		BeaconBlockRoot: beaconBlockRoot[:],
		Target:          &zondpb.Checkpoint{Root: targetRoot[:], Epoch: 3},
		Source:          &zondpb.Checkpoint{Root: sourceRoot[:], Epoch: 0},
	}, nil)

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(4).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.Attestation{}),
	).Return(&zondpb.AttestResponse{}, nil /* error */)

	validator.SubmitAttestation(context.Background(), 30, pubKey)
	validator.SubmitAttestation(context.Background(), 30, pubKey)
	require.LogsContain(t, hook, "Failed attestation slashing protection")
}

func TestAttestToBlockHead_BlocksSurroundedAtt(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	validatorIndex := primitives.ValidatorIndex(7)
	var pubKey [dilithium.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
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
		gomock.AssignableToTypeOf(&zondpb.AttestationDataRequest{}),
	).Return(&zondpb.AttestationData{
		BeaconBlockRoot: beaconBlockRoot[:],
		Target:          &zondpb.Checkpoint{Root: targetRoot[:], Epoch: 3},
		Source:          &zondpb.Checkpoint{Root: sourceRoot[:], Epoch: 0},
	}, nil)

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(4).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.Attestation{}),
	).Return(&zondpb.AttestResponse{}, nil /* error */)

	validator.SubmitAttestation(context.Background(), 30, pubKey)
	require.LogsDoNotContain(t, hook, failedAttLocalProtectionErr)

	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.AttestationDataRequest{}),
	).Return(&zondpb.AttestationData{
		BeaconBlockRoot: bytesutil.PadTo([]byte("A"), 32),
		Target:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte("B"), 32), Epoch: 2},
		Source:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte("C"), 32), Epoch: 1},
	}, nil)

	validator.SubmitAttestation(context.Background(), 30, pubKey)
	require.LogsContain(t, hook, "Failed attestation slashing protection")
}

func TestAttestToBlockHead_DoesNotAttestBeforeDelay(t *testing.T) {
	validator, m, validatorKey, finish := setup(t)
	defer finish()

	var pubKey [dilithium.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.genesisTime = uint64(qrysmTime.Now().Unix())
	m.validatorClient.EXPECT().GetDuties(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.DutiesRequest{}),
	).Times(0)

	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.AttestationDataRequest{}),
	).Times(0)

	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.Attestation{}),
	).Return(&zondpb.AttestResponse{}, nil /* error */).Times(0)

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
	var pubKey [dilithium.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			CommitteeIndex: 5,
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		}}}

	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.AttestationDataRequest{}),
	).Return(&zondpb.AttestationData{
		BeaconBlockRoot: bytesutil.PadTo([]byte("A"), 32),
		Target:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte("B"), 32)},
		Source:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte("C"), 32), Epoch: 3},
	}, nil).Do(func(arg0, arg1 interface{}) {
		wg.Done()
	})

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(2).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.Any(),
	).Return(&zondpb.AttestResponse{}, nil).Times(1)

	validator.SubmitAttestation(context.Background(), 0, pubKey)
}

func TestAttestToBlockHead_CorrectBitfieldLength(t *testing.T) {
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	validatorIndex := primitives.ValidatorIndex(2)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	var pubKey [dilithium.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			CommitteeIndex: 5,
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		}}}
	m.validatorClient.EXPECT().GetAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.AttestationDataRequest{}),
	).Return(&zondpb.AttestationData{
		Target:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte("B"), 32)},
		Source:          &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte("C"), 32), Epoch: 3},
		BeaconBlockRoot: make([]byte, fieldparams.RootLength),
	}, nil)

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(2).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	var generatedAttestation *zondpb.Attestation
	m.validatorClient.EXPECT().ProposeAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.Attestation{}),
	).Do(func(_ context.Context, att *zondpb.Attestation) {
		generatedAttestation = att
	}).Return(&zondpb.AttestResponse{}, nil /* error */)

	validator.SubmitAttestation(context.Background(), 30, pubKey)

	assert.Equal(t, 2, len(generatedAttestation.AggregationBits))
}

func TestSignAttestation(t *testing.T) {
	validator, m, _, finish := setup(t)
	defer finish()
	wantedFork := &zondpb.Fork{
		PreviousVersion: []byte{'a', 'b', 'c', 'd'},
		CurrentVersion:  []byte{'d', 'e', 'f', 'f'},
		Epoch:           0,
	}
	genesisValidatorsRoot := [32]byte{0x01, 0x02}
	attesterDomain, err := signing.Domain(wantedFork, 0, params.BeaconConfig().DomainBeaconAttester, genesisValidatorsRoot[:])
	require.NoError(t, err)
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&zondpb.DomainResponse{SignatureDomain: attesterDomain}, nil)
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
	require.Equal(t, "5a2fcf988120d65714b30aca82c84f43fb549da51f39b8979c9c2c964fc8f88a1348ae3c51bc0a27034ce9b4e3a8450f4a110a17fcae67a1dafe93e2679c10f67a36be120b29cac7a72d5ca7849d610e971f9450496a99ecc711c782afdcf8fa0b9354a1a349325b1e3c653a9c1034af33cba397a4c942bb77d3fcf990eb7ac2b4e6d36127e35c834370390ff63901e0918ae41fb421ad541d7df78544a0abab1e08fc8a0e04a2704aebda2acf78fb2de510faebe3b23e567fc72c9c6ffb485a9ef1d45fe64941c45690ebacda1eaf87ba4de6065843ee288070cebb6e5bedf45db13168f93a6ccf94211653de5ea069aa47626aff1440e3c7a50357d4d5f0367b65ef2d3c849028ef7bf25fb2ec9efb4193f397a56890116f7f08fc1c748dad005f84209d89f1ad8ad205bb45e8e03c7f9f526a4376d374b0bcdb76f9954a26783c85892ae1fb3cda5461ad713e2bc8ada2d1b265711285ca60f84bbd6b4e40b861d187cf3acb0c07ccab8e3351e2bac638e9605a7d84c2ae4061818f0e9e1646a328315c8f477273209f57171859fef377fb7d8d0b5caf2f740a4e1713725a6b32d25ef73ded2fef4b83b9f0c5810b1ad2a6e6aa7365e2bba9dcc029df0413a052ba2b17ccb0bbf114a397263dbe70bda0e47e3f17325e930a554494311c22c1045bc4b495b9044b4416b8b2a114ea4bbb2171d7ceb09c1eac9fb95c08e9c25901178d59477d6c38d852e55dd9fd2e85b175f32f6b033190b16fc62bf931e9c054007bedcb2b33d97ec26ce747785c9623d6bd64554bff45b9456963c7ebcbcc226ec139e1a362e979d75191ca08b0b2063410330c63ce873d8c7f39e01b172ae8e98751b176199229fe09d81c47a0cc48dab21ac19ccfb447bf79654e2449b51c8a2633c2f1cb6a088e368db4fef0ebc9304017dd47538fe64dbec201b0639b514705de2f251db5fbc39834e2e0d9a6156e56a0400861d832057cae490961038683b8c129b9a69ed53e61f912b61f60992d81a47d14a4ac4e3c6a38604556f8df4db1e76b0e4446e1ce0cff931e82c6e84b5419f6d42f871473b527a05985f3f364346af7a54cf99901f278a9ac6361f1282455f75cd72d8bd326aef53faf1985a49a56cae063223f66a0aa75eb2492b09741b5d556e569fa4a8621f3d6d8ad168529f9b5fdc8c5b14f3ee79b1b42800d478b85a092d6cb6468e594f3dfe74c6cb1f2a923861645e8b62774bd15dceba8100c8d0e645478b366b3dd257a0b0448459be3dd340cecb10da813d8307238b12ff95b01fbe35e5faa2643646a8c465ae0877249622f1c3245d63fbd28edb21e5f1ccd3811a185159c37664f8d7eb8e0808c6546883a0ac24239cec5b4db23606de644520ffc116c68935a8f4598774f7910b4a30a5854840f30999d4746f8f6ce08318ab89dd423ce16d9ed7a298bf44702c225f05e8fbf48c1497802b643e9c0ad9d7639cadea12128be650e3cd7d10a4c2b22334b4689a615fd0093a8eeb1558f36e2b0921eb0ded6f39e4434d702ca917ca443fd4d8d5e59b2d45c2d5c42cc44cd756f20259c7c06294a0a1596cd3f87d6b056069deb72b560450ad6783a0de0b26758f55dbecf008a4e1facee7c6f07f25cc42d365841c4585f9e02503aafa426ac85b94ca76abe0794b42531a3cec44a6d4ad6739c8af7287bf2ab71676d8a5ecbfbea44dd8e889157427b082ac213016a57a74e2c43d684878fddd4808755a1db1e4afb2ad6b8eff10237efed95530c5df40964f1b8a4b7f587fe5e44751503978c99f43b228d27428a7db665afd744cbebe7cad18108d15ba441282c4bc8abaa1b69dc5fae2b0f0201667fd6cb89c9b0a2dcd1dc28b7fdfce7aa90e977e55c5ababf1fe7802ee2e0674bebfcc6120a3a9256523126840e356c4edb95f784e53b96418ba4c253253684a328412c53811e6c65b7a9af5f6e27a47c49d9e7f3990cdf10765ac48a77e9c9937acd22b57a72ed23bf6b9c0ec65c724baa4f22e7d18c0f2d2043b98590a6bf8415ebb9c8537e287595a92d6d89211878ddd1c872bfd7b4d2c42f1ac1c08f87bdc726e78ed61c483630c259201844cff55307b0801d870996c278013a247591c2a36fa1e6c5b419b105467cf55f17abf2eff48d35e1bcb4cae5bbbbaabc43ad4c957b0c2c2ea52a251385b6443794f66b002c49feb6275f1474d56d84f05bf6d92fad1dc92c687711812f548edfbd2e7e84e6186e9d003a1a79b8cb7b3e04fa665fe826d80f485ed055b93f4e6e190b4cfd8cfddc0ef4b1578743ca7ce5a6ec7a5626536a8567494c3756fcae8a73cee6e1aa767167278de9ae88dfc4023d2a56e3a33b351b0db33992b2899e468569d76ca05437151255e58a0be277e4cc9f44b0b8a70ad6d76bee36efe512e14c1188c6f4dd8a6014917b482000e562c7048ab589ec316701c5e9ebf57aa027eeb4bb28dcaf511c02182fd3c61a074059b3b5d5a1c7c1dc40a1a44f946fed04aecc062f3df100e0de7342c1c619f4df9c5a60054a79e136c042cb5361f0af083d958e084e2e52cad2800fce617d82c8d89196f061a3869283317ddffa040bf27ed28765f898a447c76994fbeb847871c8c39bc4c5241aeec12ef5ab9eec5a7be1089de186a36065301a503b68de506a678b912c43fc2b79db95dc826b7cb250c67c5d05cc8775321be3727837bd33e235875a03dffebea8387a2def1a28f8adb2efc1b47042b827677a89277b80582cbca116f6469d9262e5be0fff72c7a55da658576e3623efdfe350327675fb4b19f071005ae43ecc876a92bfc097474dfd85b5a9b6fd797323b057cd8e64c8808e05686e14cdbd5b16baed63397b2c7b44cbbc8e9d5a441a50d3c4dbe3a27e5e02915333307144a5164c93635f7ccbc10cd65d69845c0a50c0f508aa445cb6918abe941388d03328920462e580794a4659ae33fc0e91974f5ec87bd7a4b634acee21712d7b44a9d5b82af88e59bb2c8b624fe3eceed828b07a97dc0f794c290da85d38ec01d60aced1c0cc9e6a230cd36b583c4c9955109da01bf94a1d669d46a5f3d1735ad8f86cd3fdd817f987b1e4b3fa91df55c7cf9cfd45bc7f7254e94d32e049dfda92af1471a95d7f01c8c4bcca25c51fe52ba2a4d00f88f3c1f98bc3bbb3880b6fe1fa2b6602ef2bd55a4ef1f5c7aa69230c0fe75b4746e1654467d477b8f004f90fee4ec5f761b2f688405e2d18c7e0d2f9e7ed8ba1befd8213c78076e0a84d699cdd2f6531aa1b8d065486bfb06c0dc11efbb51c55d1914b2e0ab1f5aeb1085132f3569bb674486458f100d1ef139b71606822edc9c3efcf08684e3aa0cc57f3f5f4bacdfd37b98f14a8648ee4e25b517062e44aecc867036b8816bb33a3ea68abb128b1d57263e6ce35d104e8ed94b302f392554b57dd2582667cc570d8f8d36de6a7c357fa41acddc8493aa1d8f6959297fd40fb022da0158c6b1ce35c1f43b5ca878b6757247fc2b0ca22f63dc17c55fcd1976cfdca1cd44a12b39d4a4ee4c9633d2f1634eb5ce29ad3c542ca3de7735d2e64c2c60dffdb4894060417fb085566c3d0e1956a087b1ea70c06cc52b96ecd1b01bef12571fe74de47fc088e3fd2bbd4ad5e8c424b32644d5a4a57dc519ede0052705e6f6b9a62600bbd47d5749a0941c27cfafee5082006df783da08fd1479de0421acb44a1f736a03dff85d76ba792008c548f5431d5480235074eedb15388747f434dd2d48cc5bc10c16d733317742d5c76e80b9faddd3bb19e90e6845f6121470cae7c010e0128d427bc768635f83d8a7c7ba4714f5c60d6b9b3fa6446dda81822f461c7298deec4cab2507d0d7df6ada08bbb2a7d4a68f9e641724d735192a0d36a6bd7293ad9dd048c3f7c077628f05a9752a4441af508b287dc14017b48f851d7435ff8aa5afda644462a2805152f7f6a66821f73540862c18c692a7c98238d779915ef07a4bd12c4686ab436ab5dad1094656e8bf7c159421992ca19d26da30b808a908302bfc4e26ee23660aea303cf01a6de268868bb7d78e06b26f38a462dd2564d35e576be70d414d9645be4b0abe2a8e198d5c6a38e0c98717ac581d8a8b93092f0824721aba6d6b213f3c88e14bd58b67b3e4aa7dfbf21475b2ddcbc8eb6255ab8cdf4b4acf49c38b098637f2c5f19198a7d07ee36fd96b89fd3526e94cba41586bb7eab0f92ad9e328eb9711703ca24403dfb8973d024ed4884d19312069ec665e63de477737732bb66464cf4581818ab91d71712b01f185838c286e2ec193b0a7ccef0dc3719bab6b9d9e435d4ea6ae8be2df94acf66ed2e8a6196e1f7c4c4ecdf14ea0edef60d46c0097681332f2d1001f3618e9617fd05de171442c96be98d0aecd0e00de06ebdb056a22423ae85f56a9b839845afd0045df7a219d0d8454f60d8f8b3e11296b86bcec53caa6fbac894d85f430ac749ccbad34e9a68ac7f01291a1d9a2af1be10b0521d4f7c307cacb04ad961412cd3a018e62d3fa6a561349755e1fe05df6a09a065e30b4400e8e5728a50e1daf9fa27e88df9973d706cfd493192416858d0695d60c1527556cf3edbd9e523e2f51d206327b3326d5ffd5658ec0036e8cde6446ab211d10bc9bc68f13b2b309c4c4694bf716214e5db0873d9e18c774a76744ad092bdb52af9ed6a3ea60ef66b1336a2c443b20f4083088f8bbedea16adda00a0042e31b235225c37b92351235824b95f81d8489edc7bd0207225269dc48eb7425b2a53ee4748bbc87131c16c37f5e5eb6d590cae2c05ad0cf9d83b79d498b385838bd5503c6f119cd284e16ddad23bfb1b1def29b5a6e4c4b9df2b0eb68b4d8c5f361bb2c6258a9151bb388887e8cbc3bc9265522e4773f1f731c83ff9ea93b37b9fe2134b7b8698edee6b46306d616b68156fcd9d99387b16c8822626015a59e49a8ac7e862380f354eacd97a8d1d38ab4c069549ec6dc3588a582506d2cde94b8a5046961588eede95dc0f6e0aa5b390cc4b2caa6a48c59a15863c24707edbc09524497eeaec1a2f975f48d9c5eab7e740644ac255ea806a45d71d4cbe35cb18c5bc5a6df09c9b6873fbea979fe58dbc604ab5e7234820c9dca53c89867bb2e77f7483c39f21792a8140f968ee50aba637a1fa8d65c63b12ad7d480a9f498b1034ede519200b7d4ca955a324a19460ba44736bda0ebb7f0d6b6b4137e0292f3d4fa912c7ea5b9efc5a10f7a0b2d97585c78117db35ba5edfb525a8033d19f8d8dfa6cfd9093bd9519c802962b32eebacb5f2a67028b6cb44150f601283c614b64e09a97e427bec05e18b7a4d9a88a427406038096476d0d2d2eca0f6823d1c0d1983e4fc2fb4920f9e91f687f1b51c084ccf10647158830bed513bb0f30dcd6bcb7e1ae06b83a58b381ea4b6c52188ca0661eed0ead4a6672ae7b9258134c37545f4a3debe17e5acd0eac74fd4b9219a1f9ab520ceae4339f4cb21cc6696b46af476022fc68415c686fbd3a9cb70435b94795700b5c2be67cf16b5d46cbe30b8500713a6d34aea0f333556aa7d0aa464bcd8dfab05e4b0ee8fe2cee656042bbff16345685e768e3e10a953e9b8ceee00d80ffbeb54f1353506c47ae6898875e35ba181f0edf7091d667a99c43033f2d6db604b6d6ce93844e0f1941d96f01621a165fc10740a0b1561929df63d877ab3cf30b0ee7f196ca45373e0c326845467e41510637272475d2c0052d175bfb6f5e2a0950b051a6c19075c9b35553f9e9e52ac1c70ac92786cdf8467cd2275f819e8183e857f70e8871f84293f1b6cae83d6eea3a27ef59066fbfe477daac29c14b3f0898ff88ca580d5cfed1b12e5901db4b940c869f862da2a05352ccea3382ffb4056d073611b9f454e24d7d894c5c1e0e5f1eb37c436927bbe25241366814a91818ddea90aafda22754aab2d8d9a78c0acc8bb460ad1cad5b75aae254f05ffd22d528ee05f36719a3e56a38920f39ea345d6e645c108e8b8f28f131d49e82b76182d80f22cbe1aefcd482cd236961a742a061204e031c28961ee24112f568f3a9354a5f4fe4845ac74e41b5fff395f2b482ffec8d4a55f7aca241732f5abe367d8547ec1cca8d87849fcbed4e213490c4eb477c271a2faa642c1906708ba72d6f4dcb9e91b61107fc1b92c54720ef2e277cd74cb3a4a7d64dc92b6d5a6ec212a559101458cc90aef0411b22a9867d8a892dde8a78fd472151428146ea29f667bbf8e12eaa1d8b075108df91d484549e9f83560caf4cc8d9356b593a90c9a042a42680df5062019486ca9df5a0882ad941bdfeb2bf35ee5905dafed10c82b7c5f13d6c49e21a7d5968f0e6d8e521c1ba755e947f190585d31109fb30b0d2498623e7d701ec41a943bef64d666791bbddf407212b40425b8cc9d00f476899dbecf9091c7073898acdde2045b0ef07353e54badcf30e3a6d71d619243249547d8bc2d5edf2f4000000000000000000000000000000000710171f232a2f3b", hex.EncodeToString(sig))
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
	wg.Add(1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		wsb, err := blocks.NewSignedBeaconBlock(
			&zondpb.SignedBeaconBlock{
				Block: &zondpb.BeaconBlock{Slot: currentSlot, Body: &zondpb.BeaconBlockBody{}},
			})
		require.NoError(t, err)
		v.blockFeed.Send(wsb)
		wg.Done()
	}()

	v.waitOneThirdOrValidBlock(context.Background(), currentSlot)

	if currentTime != uint64(time.Now().Unix()) {
		t.Errorf("Wanted %d time for slot one third but got %d", uint64(time.Now().Unix()), currentTime)
	}
}
