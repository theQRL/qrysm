package sync

import (
	"context"
	"testing"

	mock "github.com/cyyber/qrysm/v4/beacon-chain/blockchain/testing"
	"github.com/cyyber/qrysm/v4/beacon-chain/operations/attestations"
	lruwrpr "github.com/cyyber/qrysm/v4/cache/lru"
	fieldparams "github.com/cyyber/qrysm/v4/config/fieldparams"
	ethpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/cyyber/qrysm/v4/testing/assert"
	"github.com/cyyber/qrysm/v4/testing/require"
	"github.com/cyyber/qrysm/v4/testing/util"
	"github.com/prysmaticlabs/go-bitfield"
)

func TestBeaconAggregateProofSubscriber_CanSaveAggregatedAttestation(t *testing.T) {
	r := &Service{
		cfg: &config{
			attPool:             attestations.NewPool(),
			attestationNotifier: (&mock.ChainService{}).OperationNotifier(),
		},
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
	}

	a := &ethpb.SignedAggregateAttestationAndProof{
		Message: &ethpb.AggregateAttestationAndProof{
			Aggregate: util.HydrateAttestation(&ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x07},
			}),
			AggregatorIndex: 100,
		},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}
	require.NoError(t, r.beaconAggregateProofSubscriber(context.Background(), a))
	assert.DeepSSZEqual(t, []*ethpb.Attestation{a.Message.Aggregate}, r.cfg.attPool.AggregatedAttestations(), "Did not save aggregated attestation")
}

func TestBeaconAggregateProofSubscriber_CanSaveUnaggregatedAttestation(t *testing.T) {
	r := &Service{
		cfg: &config{
			attPool:             attestations.NewPool(),
			attestationNotifier: (&mock.ChainService{}).OperationNotifier(),
		},
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
	}

	a := &ethpb.SignedAggregateAttestationAndProof{
		Message: &ethpb.AggregateAttestationAndProof{
			Aggregate: util.HydrateAttestation(&ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x03},
				Signature:       make([]byte, fieldparams.BLSSignatureLength),
			}),
			AggregatorIndex: 100,
		},
	}
	require.NoError(t, r.beaconAggregateProofSubscriber(context.Background(), a))

	atts, err := r.cfg.attPool.UnaggregatedAttestations()
	require.NoError(t, err)
	assert.DeepEqual(t, []*ethpb.Attestation{a.Message.Aggregate}, atts, "Did not save unaggregated attestation")
}
