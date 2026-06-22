package kv_test

import (
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/operations/attestations/kv"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
)

func BenchmarkAttCaches(b *testing.B) {
	ac := kv.NewAttCaches()

	att := &qrysmpb.Attestation{}

	for b.Loop() {
		assert.NoError(b, ac.SaveUnaggregatedAttestation(att))
		assert.NoError(b, ac.DeleteAggregatedAttestation(att))
	}
}
