package blocks_test

import (
	"testing"

	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

// Blocks reconstructed from protobuf may lack nested objects that SSZ decoding
// always allocates. The generated hasher dereferences them, so the wrapper
// must refuse to hash such a block instead of panicking.
func TestHashTreeRoot_RejectsMissingNestedObjects(t *testing.T) {
	shapes := []struct {
		name  string
		apply func(b *qrysmpb.SignedBeaconBlockZond)
	}{
		{"nil execution data", func(b *qrysmpb.SignedBeaconBlockZond) { b.Block.Body.ExecutionData = nil }},
		{"nil sync aggregate", func(b *qrysmpb.SignedBeaconBlockZond) { b.Block.Body.SyncAggregate = nil }},
		{"nil execution payload", func(b *qrysmpb.SignedBeaconBlockZond) { b.Block.Body.ExecutionPayload = nil }},
		{"nil withdrawal element", func(b *qrysmpb.SignedBeaconBlockZond) {
			b.Block.Body.ExecutionPayload.Withdrawals = []*enginev1.Withdrawal{nil}
		}},
		{"empty proposer slashing", func(b *qrysmpb.SignedBeaconBlockZond) {
			b.Block.Body.ProposerSlashings = []*qrysmpb.ProposerSlashing{{}}
		}},
		{"nil proposer slashing element", func(b *qrysmpb.SignedBeaconBlockZond) {
			b.Block.Body.ProposerSlashings = []*qrysmpb.ProposerSlashing{nil}
		}},
		{"proposer slashing with nil header", func(b *qrysmpb.SignedBeaconBlockZond) {
			b.Block.Body.ProposerSlashings = []*qrysmpb.ProposerSlashing{{Header_1: &qrysmpb.SignedBeaconBlockHeader{}, Header_2: &qrysmpb.SignedBeaconBlockHeader{}}}
		}},
		{"empty attester slashing", func(b *qrysmpb.SignedBeaconBlockZond) {
			b.Block.Body.AttesterSlashings = []*qrysmpb.AttesterSlashing{{}}
		}},
		{"nil attester slashing element", func(b *qrysmpb.SignedBeaconBlockZond) {
			b.Block.Body.AttesterSlashings = []*qrysmpb.AttesterSlashing{nil}
		}},
		{"attester slashing with nil checkpoints", func(b *qrysmpb.SignedBeaconBlockZond) {
			att := &qrysmpb.IndexedAttestation{Data: &qrysmpb.AttestationData{}}
			b.Block.Body.AttesterSlashings = []*qrysmpb.AttesterSlashing{{Attestation_1: att, Attestation_2: att}}
		}},
		{"nil attestation element", func(b *qrysmpb.SignedBeaconBlockZond) {
			b.Block.Body.Attestations = []*qrysmpb.Attestation{nil}
		}},
		{"attestation with nil data", func(b *qrysmpb.SignedBeaconBlockZond) {
			b.Block.Body.Attestations = []*qrysmpb.Attestation{{AggregationBits: []byte{1}}}
		}},
		{"attestation with nil checkpoints", func(b *qrysmpb.SignedBeaconBlockZond) {
			b.Block.Body.Attestations = []*qrysmpb.Attestation{{AggregationBits: []byte{1}, Data: &qrysmpb.AttestationData{}}}
		}},
		{"nil deposit element", func(b *qrysmpb.SignedBeaconBlockZond) {
			b.Block.Body.Deposits = []*qrysmpb.Deposit{nil}
		}},
		{"deposit with nil data", func(b *qrysmpb.SignedBeaconBlockZond) {
			b.Block.Body.Deposits = []*qrysmpb.Deposit{{}}
		}},
		{"empty voluntary exit", func(b *qrysmpb.SignedBeaconBlockZond) {
			b.Block.Body.VoluntaryExits = []*qrysmpb.SignedVoluntaryExit{{}}
		}},
		{"nil voluntary exit element", func(b *qrysmpb.SignedBeaconBlockZond) {
			b.Block.Body.VoluntaryExits = []*qrysmpb.SignedVoluntaryExit{nil}
		}},
	}
	for _, s := range shapes {
		t.Run(s.name, func(t *testing.T) {
			pb := util.NewBeaconBlockZond()
			s.apply(pb)
			wsb, err := blocks.NewSignedBeaconBlock(pb)
			require.NoError(t, err)
			_, err = wsb.Block().HashTreeRoot()
			require.ErrorContains(t, "required for hashing", err)
			require.ErrorContains(t, "required for hashing", wsb.Block().HashTreeRootWith(ssz.NewHasher()))
			_, err = wsb.Block().Body().HashTreeRoot()
			require.ErrorContains(t, "required for hashing", err)
		})
	}

	t.Run("complete block hashes", func(t *testing.T) {
		wsb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
		require.NoError(t, err)
		_, err = wsb.Block().HashTreeRoot()
		require.NoError(t, err)
		_, err = wsb.Block().Body().HashTreeRoot()
		require.NoError(t, err)
	})
	t.Run("blinded block without payload header", func(t *testing.T) {
		pb := util.NewBlindedBeaconBlockZond()
		pb.Block.Body.ExecutionPayloadHeader = nil
		wsb, err := blocks.NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		_, err = wsb.Block().HashTreeRoot()
		require.ErrorContains(t, "required for hashing", err)
	})
}
