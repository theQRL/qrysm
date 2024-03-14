package validator

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	synccontribution "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation/aggregation/sync_contribution"
	"go.opencensus.io/trace"
)

func (vs *Server) setSyncAggregate(ctx context.Context, blk interfaces.SignedBeaconBlock) {
	syncAggregate, err := vs.getSyncAggregate(ctx, blk.Block().Slot()-1, blk.Block().ParentRoot())
	if err != nil {
		log.WithError(err).Error("Could not get sync aggregate")
		emptyAggregate := &zondpb.SyncAggregate{
			SyncCommitteeBits:       make([]byte, params.BeaconConfig().SyncCommitteeSize/8),
			SyncCommitteeSignatures: [][]byte{},
		}
		if err := blk.SetSyncAggregate(emptyAggregate); err != nil {
			log.WithError(err).Error("Could not set sync aggregate")
		}
		return
	}

	// Can not error. We already filter block versioning at the top. Phase 0 is impossible.
	if err := blk.SetSyncAggregate(syncAggregate); err != nil {
		log.WithError(err).Error("Could not set sync aggregate")
	}
}

// getSyncAggregate retrieves the sync contributions from the pool to construct the sync aggregate object.
// The contributions are filtered based on matching of the input root and slot then profitability.
func (vs *Server) getSyncAggregate(ctx context.Context, slot primitives.Slot, root [32]byte) (*zondpb.SyncAggregate, error) {
	_, span := trace.StartSpan(ctx, "ProposerServer.getSyncAggregate")
	defer span.End()

	if vs.SyncCommitteePool == nil {
		return nil, errors.New("sync committee pool is nil")
	}
	// Contributions have to match the input root
	contributions, err := vs.SyncCommitteePool.SyncCommitteeContributions(slot)
	if err != nil {
		return nil, err
	}
	proposerContributions := proposerSyncContributions(contributions).filterByBlockRoot(root)

	// Each sync subcommittee is 128 bits and the sync committee is 512 bits for mainnet.
	var bitsHolder [][]byte
	for i := uint64(0); i < params.BeaconConfig().SyncCommitteeSubnetCount; i++ {
		bitsHolder = append(bitsHolder, zondpb.NewSyncCommitteeAggregationBits())
	}
	sigsHolder := make([][]byte, 0, params.BeaconConfig().SyncCommitteeSize/params.BeaconConfig().SyncCommitteeSubnetCount)

	for i := uint64(0); i < params.BeaconConfig().SyncCommitteeSubnetCount; i++ {
		cs := proposerContributions.filterBySubIndex(i)
		aggregates, err := synccontribution.Aggregate(cs)
		if err != nil {
			return nil, err
		}

		// Retrieve the most profitable contribution
		deduped, err := proposerSyncContributions(aggregates).dedup()
		if err != nil {
			return nil, err
		}
		c := deduped.mostProfitable()
		if c == nil {
			continue
		}

		bitsHolder[i] = c.AggregationBits
		sigsHolder = append(sigsHolder, c.Signatures...)
	}

	// Aggregate all the contribution bits.
	var syncBits []byte
	for _, b := range bitsHolder {
		syncBits = append(syncBits, b...)
	}

	return &zondpb.SyncAggregate{
		SyncCommitteeBits:       syncBits,
		SyncCommitteeSignatures: sigsHolder,
	}, nil
}
