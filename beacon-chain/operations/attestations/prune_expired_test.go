package attestations

import (
	"context"
	"testing"
	"time"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/async"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	qrysmTime "github.com/theQRL/qrysm/time"
)

func TestPruneExpired_Ticker(t *testing.T) {
	// pruneAttsPool now fires near the end of each slot via SlotTickerWithOffset.
	// Shrink SecondsPerSlot so the test runs in seconds, not minutes — but keep it
	// large enough that the window where slot 0 is expired and slot 1 is not is
	// wider than the ~500ms polling cadence below.
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SecondsPerSlot = 5
	params.OverrideBeaconConfig(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	s, err := NewService(ctx, &Config{
		Pool: NewPool(),
	})
	require.NoError(t, err)

	ad1 := util.HydrateAttestationData(&qrysmpb.AttestationData{})

	ad2 := util.HydrateAttestationData(&qrysmpb.AttestationData{Slot: 1})

	atts := []*qrysmpb.Attestation{
		{Data: ad1, AggregationBits: bitfield.Bitlist{0b1000, 0b1}, Signatures: [][]byte{make([]byte, field_params.MLDSA87SignatureLength)}},
		{Data: ad2, AggregationBits: bitfield.Bitlist{0b1000, 0b1}, Signatures: [][]byte{make([]byte, field_params.MLDSA87SignatureLength)}},
	}
	require.NoError(t, s.cfg.Pool.SaveUnaggregatedAttestations(atts))
	require.Equal(t, 2, s.cfg.Pool.UnaggregatedAttestationCount(), "Unexpected number of attestations")
	atts = []*qrysmpb.Attestation{
		{Data: ad1, AggregationBits: bitfield.Bitlist{0b1101, 0b1}, Signatures: [][]byte{make([]byte, field_params.MLDSA87SignatureLength), make([]byte, field_params.MLDSA87SignatureLength), make([]byte, field_params.MLDSA87SignatureLength)}},
		{Data: ad2, AggregationBits: bitfield.Bitlist{0b1101, 0b1}, Signatures: [][]byte{make([]byte, field_params.MLDSA87SignatureLength), make([]byte, field_params.MLDSA87SignatureLength), make([]byte, field_params.MLDSA87SignatureLength)}},
	}
	require.NoError(t, s.cfg.Pool.SaveAggregatedAttestations(atts))
	assert.Equal(t, 2, s.cfg.Pool.AggregatedAttestationCount())
	for _, att := range atts {
		require.NoError(t, s.cfg.Pool.SaveBlockAttestation(att))
	}

	// Rewind back two epochs worth of time to cover the EIP-7045 inclusion window.
	s.genesisTime = uint64(qrysmTime.Now().Unix()) - uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot*2))

	go s.pruneAttsPool()

	done := make(chan struct{}, 1)
	async.RunEvery(ctx, 500*time.Millisecond, func() {
		atts, err := s.cfg.Pool.UnaggregatedAttestations()
		require.NoError(t, err)
		for _, attestation := range atts {
			if attestation.Data.Slot == 0 {
				return
			}
		}
		for _, attestation := range s.cfg.Pool.AggregatedAttestations() {
			if attestation.Data.Slot == 0 {
				return
			}
		}
		for _, attestation := range s.cfg.Pool.BlockAttestations() {
			if attestation.Data.Slot == 0 {
				return
			}
		}
		if s.cfg.Pool.UnaggregatedAttestationCount() != 1 || s.cfg.Pool.AggregatedAttestationCount() != 1 {
			return
		}
		done <- struct{}{}
	})
	select {
	case <-done:
		// All checks are passed.
	case <-ctx.Done():
		t.Error("Test case takes too long to complete")
	}
}

func TestPruneExpired_PruneExpiredAtts(t *testing.T) {
	s, err := NewService(context.Background(), &Config{Pool: NewPool()})
	require.NoError(t, err)

	ad1 := util.HydrateAttestationData(&qrysmpb.AttestationData{})

	ad2 := util.HydrateAttestationData(&qrysmpb.AttestationData{})

	att1 := &qrysmpb.Attestation{Data: ad1, AggregationBits: bitfield.Bitlist{0b1101}}
	att2 := &qrysmpb.Attestation{Data: ad1, AggregationBits: bitfield.Bitlist{0b1111}}
	att3 := &qrysmpb.Attestation{Data: ad2, AggregationBits: bitfield.Bitlist{0b1101}}
	att4 := &qrysmpb.Attestation{Data: ad2, AggregationBits: bitfield.Bitlist{0b1110}}
	atts := []*qrysmpb.Attestation{att1, att2, att3, att4}
	require.NoError(t, s.cfg.Pool.SaveAggregatedAttestations(atts))
	for _, att := range atts {
		require.NoError(t, s.cfg.Pool.SaveBlockAttestation(att))
	}

	// Rewind back two epochs worth of time to cover the EIP-7045 inclusion window.
	s.genesisTime = uint64(qrysmTime.Now().Unix()) - uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot*2))

	s.pruneExpiredAtts()
	// All the attestations on slot 0 should be pruned.
	for _, attestation := range s.cfg.Pool.AggregatedAttestations() {
		if attestation.Data.Slot == 0 {
			t.Error("Should be pruned")
		}
	}
	for _, attestation := range s.cfg.Pool.BlockAttestations() {
		if attestation.Data.Slot == 0 {
			t.Error("Should be pruned")
		}
	}
}

func TestPruneExpired_Expired(t *testing.T) {
	s, err := NewService(context.Background(), &Config{Pool: NewPool()})
	require.NoError(t, err)

	// Rewind back two epochs worth of time to cover the EIP-7045 inclusion window.
	s.genesisTime = uint64(qrysmTime.Now().Unix()) - uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot*2))
	assert.Equal(t, true, s.expired(0), "Should be expired")
	assert.Equal(t, false, s.expired(1), "Should not be expired")
}
