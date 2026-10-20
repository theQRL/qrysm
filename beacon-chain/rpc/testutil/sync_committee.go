package testutil

import (
	"testing"
	"time"

	fssz "github.com/prysmaticlabs/fastssz"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	p2ptypes "github.com/theQRL/qrysm/beacon-chain/p2p/types"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

// SyncCommitteeFixture provides real signatures and a mock head for RPC tests.
// The mock assigns the supplied key to each validator and committee position.
type SyncCommitteeFixture struct {
	Head *mock.ChainService
	Key  ml_dsa_87.MLDSA87Key
	Slot primitives.Slot
}

func NewSyncCommitteeFixture(t *testing.T, key ml_dsa_87.MLDSA87Key, slot primitives.Slot) *SyncCommitteeFixture {
	t.Helper()
	if key == nil {
		var err error
		key, err = ml_dsa_87.RandKey()
		require.NoError(t, err)
	}
	cfg := params.BeaconConfig()
	domain := func(kind [4]byte) []byte {
		d, err := signing.ComputeDomain(kind, cfg.GenesisForkVersion, make([]byte, 32))
		require.NoError(t, err)
		return d
	}
	pubkey := key.PublicKey().Marshal()
	head := &mock.ChainService{
		Genesis:                     time.Now().Add(-time.Duration(uint64(slot)*cfg.SecondsPerSlot+cfg.SecondsPerSlot/2) * time.Second),
		PublicKey:                   bytesutil.ToBytes2592(pubkey),
		SyncCommitteeIndices:        []primitives.CommitteeIndex{0},
		SyncCommitteePubkeys:        make([][]byte, cfg.SyncCommitteeSize/cfg.SyncCommitteeSubnetCount),
		SyncCommitteeDomain:         domain(cfg.DomainSyncCommittee),
		SyncSelectionProofDomain:    domain(cfg.DomainSyncCommitteeSelectionProof),
		SyncContributionProofDomain: domain(cfg.DomainContributionAndProof),
	}
	for i := range head.SyncCommitteePubkeys {
		head.SyncCommitteePubkeys[i] = pubkey
	}
	return &SyncCommitteeFixture{Head: head, Key: key, Slot: slot}
}

func (f *SyncCommitteeFixture) Sign(t *testing.T, object fssz.HashRoot, domain []byte) []byte {
	t.Helper()
	root, err := signing.ComputeSigningRoot(object, domain)
	require.NoError(t, err)
	sig, err := f.Key.Sign(root[:])
	require.NoError(t, err)
	return sig.Marshal()
}

func (f *SyncCommitteeFixture) Message(t *testing.T, index primitives.ValidatorIndex, root []byte) *qrysmpb.SyncCommitteeMessage {
	t.Helper()
	obj := p2ptypes.SSZBytes(root)
	return &qrysmpb.SyncCommitteeMessage{
		Slot: f.Slot, ValidatorIndex: index, BlockRoot: bytesutil.SafeCopyBytes(root),
		Signature: f.Sign(t, &obj, f.Head.SyncCommitteeDomain),
	}
}

func (f *SyncCommitteeFixture) Contribution(t *testing.T) *qrysmpb.SignedContributionAndProof {
	t.Helper()
	var aggregator primitives.ValidatorIndex
	for ; ; aggregator++ {
		selected, err := altair.IsSyncCommitteeAggregator(f.Head.AggregatorSelectionSeed[:], f.Slot, 0, aggregator)
		require.NoError(t, err)
		if selected {
			break
		}
		require.Equal(t, true, aggregator < 1024, "could not find an aggregator")
	}
	msg := f.Message(t, aggregator, make([]byte, 32))
	bits := qrysmpb.NewSyncCommitteeAggregationBits()
	bits.SetBitAt(0, true)
	selection := &qrysmpb.SyncAggregatorSelectionData{Slot: f.Slot, SubcommitteeIndex: 0}
	proof := &qrysmpb.ContributionAndProof{
		AggregatorIndex: aggregator,
		Contribution: &qrysmpb.SyncCommitteeContribution{
			Slot: f.Slot, BlockRoot: msg.BlockRoot, AggregationBits: bits, Signatures: [][]byte{msg.Signature},
		},
		SelectionProof: f.Sign(t, selection, f.Head.SyncSelectionProofDomain),
	}
	return &qrysmpb.SignedContributionAndProof{
		Message: proof, Signature: f.Sign(t, proof, f.Head.SyncContributionProofDomain),
	}
}
