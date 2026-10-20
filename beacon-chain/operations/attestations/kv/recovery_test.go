package kv

import (
	"context"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/config/features"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func recoveryAttestation(bits byte) *qrysmpb.Attestation {
	att := util.HydrateAttestation(&qrysmpb.Attestation{
		AggregationBits: bitfield.Bitlist{bits},
		Data:            &qrysmpb.AttestationData{Slot: 1},
	})
	signature := att.Signatures[0]
	att.Signatures = make([][]byte, att.AggregationBits.Count())
	for i := range att.Signatures {
		att.Signatures[i] = append([]byte(nil), signature...)
	}
	return att
}

func TestKV_RecoverAttestation(t *testing.T) {
	for _, aggregated := range []bool{false, true} {
		for _, queued := range []bool{false, true} {
			name := map[bool]string{false: "unaggregated", true: "aggregated"}[aggregated]
			name += map[bool]string{false: "/processed", true: "/queued for forkchoice"}[queued]
			t.Run(name, func(t *testing.T) {
				c := NewAttCaches()
				att := recoveryAttestation(0b10001)
				save, remove := c.SaveUnaggregatedAttestation, c.DeleteUnaggregatedAttestation
				if aggregated {
					att = recoveryAttestation(0b10011)
					save, remove = c.SaveAggregatedAttestation, c.DeleteAggregatedAttestation
				}
				proposalAtts := func() []*qrysmpb.Attestation {
					t.Helper()
					atts, err := c.UnaggregatedAttestations()
					require.NoError(t, err)
					return append(atts, c.AggregatedAttestations()...)
				}
				require.NoError(t, save(att))
				require.Equal(t, 1, len(proposalAtts()))
				require.NoError(t, remove(att))
				if queued {
					require.NoError(t, c.SaveBlockAttestation(att))
					require.NoError(t, c.SaveForkchoiceAttestation(att))
				}
				require.NoError(t, save(att))
				require.Equal(t, 0, len(proposalAtts()), "ordinary saves must respect canonical inclusion")

				for range 2 {
					require.NoError(t, c.RecoverAttestation(att))
					require.DeepSSZEqual(t, []*qrysmpb.Attestation{att}, proposalAtts(), "recovery retries must not duplicate the vote")
				}
				if queued {
					require.DeepSSZEqual(t, []*qrysmpb.Attestation{att}, c.BlockAttestations())
					require.DeepSSZEqual(t, []*qrysmpb.Attestation{att}, c.ForkchoiceAttestations())
					require.NoError(t, c.DeleteBlockAttestation(att))
					require.NoError(t, c.DeleteForkchoiceAttestation(att))
					require.DeepSSZEqual(t, []*qrysmpb.Attestation{att}, proposalAtts(), "processing forkchoice votes must preserve proposal candidates")
				}

				// Once included again, the recovered vote obeys normal deduplication.
				require.NoError(t, remove(att))
				require.NoError(t, save(att))
				require.Equal(t, 0, len(proposalAtts()))
			})
		}
	}
}

func TestKV_RecoverAttestation_PreservesOtherParticipants(t *testing.T) {
	c := NewAttCaches()
	// Canonical pruning of a pooled aggregate marks both seen caches with the
	// orphaned participants 0 and 1, plus unrelated participants 2 and 3.
	included := recoveryAttestation(0b11111)
	require.NoError(t, c.SaveAggregatedAttestation(included))
	require.NoError(t, c.DeleteAggregatedAttestation(included))
	orphan := recoveryAttestation(0b10011)
	require.NoError(t, c.RecoverAttestation(orphan))
	other := recoveryAttestation(0b11100)
	for _, seen := range []func(*qrysmpb.Attestation) (bool, error){c.hasSeenBit, c.hasSeenAggregatedBit} {
		has, err := seen(orphan)
		require.NoError(t, err)
		require.Equal(t, false, has)
		has, err = seen(other)
		require.NoError(t, err)
		require.Equal(t, true, has, "unrelated participants remain seen")
	}
	require.NoError(t, c.SaveAggregatedAttestation(other))
	require.NoError(t, c.SaveUnaggregatedAttestation(recoveryAttestation(0b10100)))
	require.DeepSSZEqual(t, []*qrysmpb.Attestation{orphan}, c.AggregatedAttestations())
	require.Equal(t, 0, c.UnaggregatedAttestationCount())
}

func TestKV_RecoverAttestation_CanAggregate(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		for _, mode := range []string{"processed before recovery", "queued", "processed after recovery"} {
			name := map[bool]string{false: "serial", true: "parallel"}[parallel]
			t.Run(name+"/"+mode, func(t *testing.T) {
				t.Cleanup(features.InitWithReset(&features.Flags{AggregateParallel: parallel}))
				c := NewAttCaches()
				// The same participants may have been processed as an aggregate
				// before their individual votes are recovered from the orphaned branch.
				aggregate := recoveryAttestation(0b10011)
				require.NoError(t, c.SaveAggregatedAttestation(aggregate))
				require.NoError(t, c.DeleteAggregatedAttestation(aggregate))
				require.NoError(t, c.RecoverAttestation(recoveryAttestation(0b10001)))
				require.NoError(t, c.RecoverAttestation(recoveryAttestation(0b10010)))
				if mode != "processed before recovery" {
					require.NoError(t, c.SaveBlockAttestation(aggregate))
					if mode == "processed after recovery" {
						require.NoError(t, c.DeleteBlockAttestation(aggregate))
					}
				}
				require.NoError(t, c.AggregateUnaggregatedAttestations(context.Background()))
				require.Equal(t, 1, c.AggregatedAttestationCount())
				require.DeepSSZEqual(t, []*qrysmpb.Attestation{aggregate}, c.AggregatedAttestations())
				require.Equal(t, 0, c.UnaggregatedAttestationCount())
			})
		}
	}
}

func TestKV_RecoverAttestation_RejectsMalformed(t *testing.T) {
	c := NewAttCaches()
	att := recoveryAttestation(0b10011)
	require.NoError(t, c.SaveAggregatedAttestation(att))
	require.NoError(t, c.DeleteAggregatedAttestation(att))
	invalid := qrysmpb.CopyAttestation(att)
	invalid.Signatures[0] = []byte{1}
	require.NotNil(t, c.RecoverAttestation(invalid))
	require.NoError(t, c.SaveAggregatedAttestation(att))
	require.Equal(t, 0, c.AggregatedAttestationCount(), "failed recovery must not clear the inclusion history")
	require.NotNil(t, c.RecoverAttestation(nil))
}
