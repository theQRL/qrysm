package client

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-bitfield"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	"github.com/theQRL/qrysm/v4/time/slots"
)

func TestSubmitAggregateAndProof_GetDutiesRequestFailure(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, _, validatorKey, finish := setup(t)
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{}}
	defer finish()

	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.SubmitAggregateAndProof(context.Background(), 0, pubKey)

	require.LogsContain(t, hook, "Could not fetch validator assignment")
}

func TestSubmitAggregateAndProof_SignFails(t *testing.T) {
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &zondpb.DutiesResponse{
		CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
			{
				PublicKey: validatorKey.PublicKey().Marshal(),
			},
		},
	}

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	m.validatorClient.EXPECT().SubmitAggregateSelectionProof(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.AggregateSelectionRequest{}),
	).Return(&zondpb.AggregateSelectionResponse{
		AggregateAndProof: &zondpb.AggregateAttestationAndProof{
			AggregatorIndex: 0,
			Aggregate: util.HydrateAttestation(&zondpb.Attestation{
				AggregationBits: make([]byte, 1),
			}),
			SelectionProof: make([]byte, 4595),
		},
	}, nil)

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Return(&zondpb.DomainResponse{SignatureDomain: nil}, errors.New("bad domain root"))

	validator.SubmitAggregateAndProof(context.Background(), 0, pubKey)
}

func TestSubmitAggregateAndProof_Ok(t *testing.T) {
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.duties = &zondpb.DutiesResponse{
		CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
			{
				PublicKey: validatorKey.PublicKey().Marshal(),
			},
		},
	}

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	m.validatorClient.EXPECT().SubmitAggregateSelectionProof(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.AggregateSelectionRequest{}),
	).Return(&zondpb.AggregateSelectionResponse{
		AggregateAndProof: &zondpb.AggregateAttestationAndProof{
			AggregatorIndex: 0,
			Aggregate: util.HydrateAttestation(&zondpb.Attestation{
				AggregationBits: make([]byte, 1),
			}),
			SelectionProof: make([]byte, 4595),
		},
	}, nil)

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	m.validatorClient.EXPECT().SubmitSignedAggregateSelectionProof(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.SignedAggregateSubmitRequest{}),
	).Return(&zondpb.SignedAggregateSubmitResponse{AttestationDataRoot: make([]byte, 32)}, nil)

	validator.SubmitAggregateAndProof(context.Background(), 0, pubKey)
}

func TestWaitForSlotTwoThird_WaitCorrectly(t *testing.T) {
	cfg := params.BeaconConfig().Copy()
	fmt.Println(cfg.SecondsPerSlot)
	cfg.SecondsPerSlot = 12
	params.OverrideBeaconConfig(cfg)

	validator, _, _, finish := setup(t)
	defer finish()
	currentTime := time.Now()
	numOfSlots := primitives.Slot(4)
	validator.genesisTime = uint64(currentTime.Unix()) - uint64(numOfSlots.Mul(params.BeaconConfig().SecondsPerSlot))
	oneThird := slots.DivideSlotBy(3 /* one third of slot duration */)
	timeToSleep := oneThird + oneThird

	twoThirdTime := currentTime.Add(timeToSleep)
	validator.waitToSlotTwoThirds(context.Background(), numOfSlots)
	currentTime = time.Now()
	fmt.Println(twoThirdTime.Unix())
	fmt.Println(currentTime.Unix())
	assert.Equal(t, twoThirdTime.Unix(), time.Now().Unix())
}

func TestWaitForSlotTwoThird_DoneContext_ReturnsImmediately(t *testing.T) {
	cfg := params.BeaconConfig().Copy()
	cfg.SecondsPerSlot = 10
	params.OverrideBeaconConfig(cfg)

	validator, _, _, finish := setup(t)
	defer finish()
	currentTime := time.Now()
	numOfSlots := primitives.Slot(4)
	validator.genesisTime = uint64(currentTime.Unix()) - uint64(numOfSlots.Mul(params.BeaconConfig().SecondsPerSlot))

	expectedTime := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	validator.waitToSlotTwoThirds(ctx, numOfSlots)
	currentTime = time.Now()
	assert.Equal(t, expectedTime.Unix(), currentTime.Unix())
}

func TestAggregateAndProofSignature_CanSignValidSignature(t *testing.T) {
	validator, m, validatorKey, finish := setup(t)
	defer finish()

	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		&zondpb.DomainRequest{Epoch: 0, Domain: params.BeaconConfig().DomainAggregateAndProof[:]},
	).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	agg := &zondpb.AggregateAttestationAndProof{
		AggregatorIndex: 0,
		Aggregate: util.HydrateAttestation(&zondpb.Attestation{
			AggregationBits: bitfield.NewBitlist(1),
		}),
		SelectionProof: make([]byte, 4595),
	}
	sig, err := validator.aggregateAndProofSig(context.Background(), pubKey, agg, 0 /* slot */)
	require.NoError(t, err)
	_, err = dilithium.SignatureFromBytes(sig)
	require.NoError(t, err)
}
