package shared

import (
	"testing"

	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

// Each ToConsensus that dereferences inner pointers must reject nil receivers
// and nil sub-fields with a DecodeError instead of panicking. Regression test
// for the prysm #14867-equivalent DoS in qrysm.
func TestToConsensus_NilGuards(t *testing.T) {
	t.Run("Fork nil receiver", func(t *testing.T) {
		var s *Fork
		_, err := s.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("SignedValidatorRegistration nil receiver", func(t *testing.T) {
		var s *SignedValidatorRegistration
		_, err := s.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("SignedValidatorRegistration nil Message", func(t *testing.T) {
		_, err := (&SignedValidatorRegistration{}).ToConsensus()
		require.NotNil(t, err)
		assert.StringContains(t, "Message", err.Error())
	})
	t.Run("ValidatorRegistration nil receiver", func(t *testing.T) {
		var s *ValidatorRegistration
		_, err := s.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("SignedContributionAndProof nil receiver", func(t *testing.T) {
		var s *SignedContributionAndProof
		_, err := s.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("SignedContributionAndProof nil Message", func(t *testing.T) {
		_, err := (&SignedContributionAndProof{}).ToConsensus()
		require.NotNil(t, err)
		assert.StringContains(t, "Message", err.Error())
	})
	t.Run("ContributionAndProof nil receiver", func(t *testing.T) {
		var c *ContributionAndProof
		_, err := c.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("ContributionAndProof nil Contribution", func(t *testing.T) {
		_, err := (&ContributionAndProof{}).ToConsensus()
		require.NotNil(t, err)
		assert.StringContains(t, "Contribution", err.Error())
	})
	t.Run("SyncCommitteeContribution nil receiver", func(t *testing.T) {
		var s *SyncCommitteeContribution
		_, err := s.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("SignedAggregateAttestationAndProof nil receiver", func(t *testing.T) {
		var s *SignedAggregateAttestationAndProof
		_, err := s.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("SignedAggregateAttestationAndProof nil Message", func(t *testing.T) {
		_, err := (&SignedAggregateAttestationAndProof{}).ToConsensus()
		require.NotNil(t, err)
		assert.StringContains(t, "Message", err.Error())
	})
	t.Run("AggregateAttestationAndProof nil receiver", func(t *testing.T) {
		var a *AggregateAttestationAndProof
		_, err := a.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("AggregateAttestationAndProof nil Aggregate", func(t *testing.T) {
		_, err := (&AggregateAttestationAndProof{}).ToConsensus()
		require.NotNil(t, err)
		assert.StringContains(t, "Aggregate", err.Error())
	})
	t.Run("Attestation nil receiver", func(t *testing.T) {
		var a *Attestation
		_, err := a.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("Attestation nil Data", func(t *testing.T) {
		_, err := (&Attestation{}).ToConsensus()
		require.NotNil(t, err)
		assert.StringContains(t, "Data", err.Error())
	})
	t.Run("AttestationData nil receiver", func(t *testing.T) {
		var a *AttestationData
		_, err := a.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("AttestationData nil Source", func(t *testing.T) {
		_, err := (&AttestationData{Target: &Checkpoint{}}).ToConsensus()
		require.NotNil(t, err)
		assert.StringContains(t, "Source", err.Error())
	})
	t.Run("AttestationData nil Target", func(t *testing.T) {
		_, err := (&AttestationData{Source: &Checkpoint{}}).ToConsensus()
		require.NotNil(t, err)
		assert.StringContains(t, "Target", err.Error())
	})
	t.Run("Checkpoint nil receiver", func(t *testing.T) {
		var c *Checkpoint
		_, err := c.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("SyncCommitteeSubscription nil receiver", func(t *testing.T) {
		var s *SyncCommitteeSubscription
		_, err := s.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("BeaconCommitteeSubscription nil receiver", func(t *testing.T) {
		var b *BeaconCommitteeSubscription
		_, err := b.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("SignedVoluntaryExit nil receiver", func(t *testing.T) {
		var e *SignedVoluntaryExit
		_, err := e.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("SignedVoluntaryExit nil Message", func(t *testing.T) {
		_, err := (&SignedVoluntaryExit{}).ToConsensus()
		require.NotNil(t, err)
		assert.StringContains(t, "Message", err.Error())
	})
	t.Run("VoluntaryExit nil receiver", func(t *testing.T) {
		var e *VoluntaryExit
		_, err := e.ToConsensus()
		require.NotNil(t, err)
	})
	t.Run("SyncCommitteeMessage nil receiver", func(t *testing.T) {
		var m *SyncCommitteeMessage
		_, err := m.ToConsensus()
		require.NotNil(t, err)
	})
}
