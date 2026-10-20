package slasher

import (
	"context"

	"github.com/pkg/errors"
	slashertypes "github.com/theQRL/qrysm/beacon-chain/slasher/types"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"go.opencensus.io/trace"
)

// detectProposerSlashings takes in signed block header wrappers and returns a list of proposer slashings detected.
func (s *Service) detectProposerSlashings(
	ctx context.Context,
	incomingProposals []*slashertypes.SignedBlockHeaderWrapper,
) ([]*qrysmpb.ProposerSlashing, error) {
	ctx, span := trace.StartSpan(ctx, "slasher.detectProposerSlashings")
	defer span.End()

	// internalSlashings will contain any slashable double proposals in the input list
	// of proposals with respect to each other.
	internalSlashings := []*qrysmpb.ProposerSlashing{}

	existingProposals := make(map[string]*slashertypes.SignedBlockHeaderWrapper)
	// conflictingKeys holds every (slot, proposer) for which more than one
	// distinct header is known, in this batch or on disk.
	conflictingKeys := make(map[string]bool)

	// We check if there are any slashable double proposals in the input list
	// of proposals with respect to each other.
	for _, incomingProposal := range incomingProposals {
		key := proposalKey(incomingProposal)
		existingProposal, ok := existingProposals[key]

		// If we have not seen this proposal before, we add it to our map of existing proposals
		// and we continue to the next proposal.
		if !ok {
			existingProposals[key] = incomingProposal
			continue
		}

		// If we have seen this proposal before, we check if it is a double proposal.
		if isDoubleProposal(incomingProposal.SigningRoot, existingProposal.SigningRoot) {
			doubleProposalsTotal.Inc()

			slashing := &qrysmpb.ProposerSlashing{
				Header_1: existingProposal.SignedBeaconBlockHeader,
				Header_2: incomingProposal.SignedBeaconBlockHeader,
			}

			internalSlashings = append(internalSlashings, slashing)
			conflictingKeys[key] = true
		}
	}

	// We check if there are any slashable double proposals in the input list
	// of proposals with respect to the slasher database.
	databaseSlashings, err := s.serviceCfg.Database.CheckDoubleBlockProposals(ctx, incomingProposals)
	if err != nil {
		return nil, errors.Wrap(err, "could not check for double proposals on disk")
	}
	for _, slashing := range databaseSlashings {
		conflictingKeys[headerKey(slashing.Header_2.Header)] = true
	}

	// Proposals that conflict with nothing are saved as they are. Headers reach
	// the slasher before the sync layer has verified them, so a (slot, proposer)
	// with conflicting headers is resolved by signature verification below: an
	// unverifiable header must never replace a verified record, or the real
	// double proposal would only ever be compared against junk.
	safeProposals := make([]*slashertypes.SignedBlockHeaderWrapper, 0, len(incomingProposals))
	conflictingProposals := make([]*slashertypes.SignedBlockHeaderWrapper, 0)
	for _, incomingProposal := range incomingProposals {
		if conflictingKeys[proposalKey(incomingProposal)] {
			conflictingProposals = append(conflictingProposals, incomingProposal)
			continue
		}
		safeProposals = append(safeProposals, incomingProposal)
	}
	if err := s.serviceCfg.Database.SaveBlockProposals(ctx, safeProposals); err != nil {
		return nil, errors.Wrap(err, "could not save safe proposals")
	}
	// A proposal queued again for an unverifiable conflict can come back
	// without one (its counterpart was pruned meanwhile); its retry
	// bookkeeping must not outlive it.
	s.forgetProposalRetries(safeProposals)
	unresolved, err := s.resolveConflictingProposals(ctx, conflictingProposals)
	if err != nil {
		return nil, errors.Wrap(err, "could not resolve conflicting proposals")
	}
	s.requeueUnresolvedProposals(unresolved)

	// totalSlashings contain all slashings we have detected.
	totalSlashings := append(internalSlashings, databaseSlashings...)
	return totalSlashings, nil
}

// proposalKey build a key which is a combination of the slot and the proposer index.
// If a validator proposes several blocks for the same slot, then several (potentially slashable)
// proposals will correspond to the same key.
func proposalKey(proposal *slashertypes.SignedBlockHeaderWrapper) string {
	header := proposal.SignedBeaconBlockHeader.Header

	slotKey := uintToString(uint64(header.Slot))
	proposerIndexKey := uintToString(uint64(header.ProposerIndex))

	return slotKey + ":" + proposerIndexKey
}
