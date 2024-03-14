package sync

import (
	"context"
	"testing"

	"github.com/theQRL/go-bitfield"
	mock "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/operations/attestations"
	lruwrpr "github.com/theQRL/qrysm/v4/cache/lru"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestBeaconAggregateProofSubscriber_CanSaveAggregatedAttestation(t *testing.T) {
	r := &Service{
		cfg: &config{
			attPool:             attestations.NewPool(),
			attestationNotifier: (&mock.ChainService{}).OperationNotifier(),
		},
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
	}

	a := &zondpb.SignedAggregateAttestationAndProof{
		Message: &zondpb.AggregateAttestationAndProof{
			Aggregate: util.HydrateAttestation(&zondpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x07},
			}),
			AggregatorIndex: 100,
		},
		Signature: make([]byte, field_params.DilithiumSignatureLength),
	}
	require.NoError(t, r.beaconAggregateProofSubscriber(context.Background(), a))
	assert.DeepSSZEqual(t, []*zondpb.Attestation{a.Message.Aggregate}, r.cfg.attPool.AggregatedAttestations(), "Did not save aggregated attestation")
}

func TestBeaconAggregateProofSubscriber_CanSaveUnaggregatedAttestation(t *testing.T) {
	r := &Service{
		cfg: &config{
			attPool:             attestations.NewPool(),
			attestationNotifier: (&mock.ChainService{}).OperationNotifier(),
		},
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
	}

	a := &zondpb.SignedAggregateAttestationAndProof{
		Message: &zondpb.AggregateAttestationAndProof{
			Aggregate: util.HydrateAttestation(&zondpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x03},
				Signatures:      [][]byte{},
			}),
			AggregatorIndex: 100,
		},
	}
	require.NoError(t, r.beaconAggregateProofSubscriber(context.Background(), a))

	atts, err := r.cfg.attPool.UnaggregatedAttestations()
	require.NoError(t, err)
	assert.DeepEqual(t, []*zondpb.Attestation{a.Message.Aggregate}, atts, "Did not save unaggregated attestation")
}
