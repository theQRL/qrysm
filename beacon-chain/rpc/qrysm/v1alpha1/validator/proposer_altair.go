package validator

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	synccontribution "github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation/aggregation/sync_contribution"
	"go.opencensus.io/trace"
)

func (vs *Server) setSyncAggregate(ctx context.Context, blk interfaces.SignedBeaconBlock, headState state.BeaconState) {
	syncAggregate, err := vs.getSyncAggregate(ctx, blk.Block().Slot()-1, blk.Block().ParentRoot(), headState)
	if err != nil {
		log.WithError(err).Error("Could not get sync aggregate")
		emptyAggregate := &qrysmpb.SyncAggregate{
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
func (vs *Server) getSyncAggregate(ctx context.Context, slot primitives.Slot, root [32]byte, headState state.BeaconState) (*qrysmpb.SyncAggregate, error) {
	_, span := trace.StartSpan(ctx, "ProposerServer.getSyncAggregate")
	defer span.End()

	if vs.SyncCommitteePool == nil {
		return nil, errors.New("sync committee pool is nil")
	}

	poolContributions, err := vs.SyncCommitteePool.SyncCommitteeContributions(slot)
	if err != nil {
		return nil, err
	}
	// Contributions have to match the input root
	proposerContributions := proposerSyncContributions(poolContributions).filterByBlockRoot(root)

	aggregatedContributions, err := vs.aggregatedSyncCommitteeMessages(ctx, slot, root, poolContributions, headState)
	if err != nil {
		return nil, errors.Wrap(err, "could not get aggregated sync committee messages")
	}
	proposerContributions = append(proposerContributions, aggregatedContributions...)

	subcommitteeCount := params.BeaconConfig().SyncCommitteeSubnetCount
	var bitsHolder [][]byte
	for i := uint64(0); i < subcommitteeCount; i++ {
		bitsHolder = append(bitsHolder, qrysmpb.NewSyncCommitteeAggregationBits())
	}
	sigsHolder := make([][]byte, 0, params.BeaconConfig().SyncCommitteeSize/subcommitteeCount)

	for i := uint64(0); i < subcommitteeCount; i++ {
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

	return &qrysmpb.SyncAggregate{
		SyncCommitteeBits:       syncBits,
		SyncCommitteeSignatures: sigsHolder,
	}, nil
}

// aggregatedSyncCommitteeMessages collects unaggregated sync committee messages from the pool
// and packs them into per-subcommittee SyncCommitteeContribution objects, skipping bits already
// covered by existing pool contributions to avoid intersections that weaken the final aggregate.
func (vs *Server) aggregatedSyncCommitteeMessages(
	ctx context.Context,
	slot primitives.Slot,
	root [32]byte,
	poolContributions []*qrysmpb.SyncCommitteeContribution,
	st state.BeaconState,
) ([]*qrysmpb.SyncCommitteeContribution, error) {
	subcommitteeCount := params.BeaconConfig().SyncCommitteeSubnetCount
	subcommitteeSize := params.BeaconConfig().SyncCommitteeSize / subcommitteeCount
	sigsPerSubcommittee := make([][][]byte, subcommitteeCount)
	bitsPerSubcommittee := make([]bitfield.Bitfield, subcommitteeCount)
	for i := uint64(0); i < subcommitteeCount; i++ {
		sigsPerSubcommittee[i] = make([][]byte, 0, subcommitteeSize)
		bitsPerSubcommittee[i] = qrysmpb.NewSyncCommitteeAggregationBits()
	}

	// Get committee position(s) for each message's validator index.
	scMessages, err := vs.SyncCommitteePool.SyncCommitteeMessages(slot)
	if err != nil {
		return nil, errors.Wrap(err, "could not get sync committee messages")
	}
	messageIndices := make([]primitives.ValidatorIndex, 0, len(scMessages))
	messageSigs := make([][]byte, 0, len(scMessages))
	for _, msg := range scMessages {
		if bytes.Equal(root[:], msg.BlockRoot) {
			messageIndices = append(messageIndices, msg.ValidatorIndex)
			messageSigs = append(messageSigs, msg.Signature)
		}
	}
	if len(messageIndices) == 0 {
		return nil, nil
	}
	positions, err := helpers.CurrentPeriodPositions(st, messageIndices)
	if err != nil {
		return nil, errors.Wrap(err, "could not get sync committee positions")
	}

	// Based on committee position(s), set the appropriate subcommittee bit and signature.
	for i, ci := range positions {
		for _, index := range ci {
			k := uint64(index)
			subnetIndex := k / subcommitteeSize
			indexMod := k % subcommitteeSize

			// Existing aggregated contributions from the pool intersecting with aggregates
			// created from single sync committee messages can result in bit intersections
			// that fail to produce the best possible final aggregate. Ignoring bits that are
			// already set in pool contributions makes intersections impossible.
			intersects := false
			for _, poolContrib := range poolContributions {
				if poolContrib.SubcommitteeIndex == subnetIndex && poolContrib.AggregationBits.BitAt(indexMod) {
					intersects = true
					break
				}
			}
			if !intersects && !bitsPerSubcommittee[subnetIndex].BitAt(indexMod) {
				bitsPerSubcommittee[subnetIndex].SetBitAt(indexMod, true)
				sigsPerSubcommittee[subnetIndex] = append(sigsPerSubcommittee[subnetIndex], messageSigs[i])
			}
		}
	}

	// Pack signatures and bits into per-subcommittee contributions.
	result := make([]*qrysmpb.SyncCommitteeContribution, 0, subcommitteeCount)
	for i := uint64(0); i < subcommitteeCount; i++ {
		if len(sigsPerSubcommittee[i]) == 0 {
			continue
		}
		result = append(result, &qrysmpb.SyncCommitteeContribution{
			Slot:              slot,
			BlockRoot:         root[:],
			SubcommitteeIndex: i,
			AggregationBits:   bitsPerSubcommittee[i].Bytes(),
			Signatures:        sigsPerSubcommittee[i],
		})
	}

	return result, nil
}
