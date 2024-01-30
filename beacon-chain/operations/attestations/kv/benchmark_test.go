package kv_test

import (
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/operations/attestations/kv"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
)

func BenchmarkAttCaches(b *testing.B) {
	ac := kv.NewAttCaches()

	att := &zondpb.Attestation{}

	for i := 0; i < b.N; i++ {
		assert.NoError(b, ac.SaveUnaggregatedAttestation(att))
		assert.NoError(b, ac.DeleteAggregatedAttestation(att))
	}
}
