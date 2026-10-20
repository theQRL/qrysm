package slasher

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	slashertypes "github.com/theQRL/qrysm/beacon-chain/slasher/types"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// maxProposalRetries bounds how often a conflicting proposal whose signature
// could not be checked is queued again for the next tick.
const maxProposalRetries = 8

// headerKey builds the (slot, proposer) key of a block header.
func headerKey(header *qrysmpb.BeaconBlockHeader) string {
	return uintToString(uint64(header.Slot)) + ":" + uintToString(uint64(header.ProposerIndex))
}

// headerVerdict is the outcome of verifying a block header signature.
type headerVerdict int

const (
	// headerVerified: the signature verifies.
	headerVerified headerVerdict = iota
	// headerInvalid: the signature definitely does not verify.
	headerInvalid
	// headerInconclusive: the check could not be carried out (no head state
	// right now, or a proposer the head registry does not hold yet).
	headerInconclusive
)

// verifyHeader classifies a header signature check.
func (s *Service) verifyHeader(ctx context.Context, header *qrysmpb.SignedBeaconBlockHeader) headerVerdict {
	err := s.verifyBlockSignature(ctx, header)
	switch {
	case err == nil:
		return headerVerified
	case errors.Is(err, errHeaderVerificationUnavailable):
		log.WithError(err).WithField("slot", header.Header.Slot).WithField("proposerIndex", header.Header.ProposerIndex).
			Debug("Could not verify a conflicting block header signature; will retry")
		return headerInconclusive
	default:
		return headerInvalid
	}
}

// resolveConflictingProposals decides which header to keep on disk for every
// (slot, proposer) that has several distinct headers. A stored record whose
// signature verifies is kept. Otherwise the first incoming header that verifies
// replaces it. A header whose signature definitely fails is never stored, so
// junk fed through gossip cannot mask a real double proposal across batches.
// Headers whose signature could not be checked are returned so the caller can
// queue them again: discarding them would lose evidence to a transient failure.
func (s *Service) resolveConflictingProposals(ctx context.Context, proposals []*slashertypes.SignedBlockHeaderWrapper) ([]*slashertypes.SignedBlockHeaderWrapper, error) {
	if len(proposals) == 0 {
		return nil, nil
	}
	groups := make(map[string][]*slashertypes.SignedBlockHeaderWrapper)
	keys := make([]string, 0)
	for _, proposal := range proposals {
		key := proposalKey(proposal)
		if _, ok := groups[key]; !ok {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], proposal)
	}
	sort.Strings(keys)

	unresolved := make([]*slashertypes.SignedBlockHeaderWrapper, 0)
	for _, key := range keys {
		group := groups[key]
		header := group[0].SignedBeaconBlockHeader.Header
		stored, err := s.serviceCfg.Database.BlockProposalForValidator(ctx, header.ProposerIndex, header.Slot)
		if err != nil {
			return nil, errors.Wrap(err, "could not read the stored proposal")
		}
		if stored != nil && s.verifyHeader(ctx, stored.SignedBeaconBlockHeader) == headerVerified {
			// A verified record is never replaced; the incoming headers were
			// compared against it already.
			s.forgetProposalRetries(group)
			continue
		}
		var replacement *slashertypes.SignedBlockHeaderWrapper
		for _, proposal := range group {
			switch s.verifyHeader(ctx, proposal.SignedBeaconBlockHeader) {
			case headerVerified:
				if replacement == nil {
					replacement = proposal
				}
				delete(s.proposalRetries, proposal.SigningRoot)
			case headerInconclusive:
				unresolved = append(unresolved, proposal)
			case headerInvalid:
				delete(s.proposalRetries, proposal.SigningRoot)
			}
		}
		if replacement == nil {
			continue
		}
		if err := s.serviceCfg.Database.SaveBlockProposals(ctx, []*slashertypes.SignedBlockHeaderWrapper{replacement}); err != nil {
			return nil, errors.Wrap(err, "could not save the verified proposal")
		}
	}
	return unresolved, nil
}

// requeueUnresolvedProposals puts headers whose signature could not be checked
// back on the queue for the next tick, up to maxProposalRetries times each.
func (s *Service) requeueUnresolvedProposals(unresolved []*slashertypes.SignedBlockHeaderWrapper) {
	if len(unresolved) == 0 || s.blksQueue == nil {
		return
	}
	if s.proposalRetries == nil {
		s.proposalRetries = make(map[[32]byte]uint8)
	}
	keep := make([]*slashertypes.SignedBlockHeaderWrapper, 0, len(unresolved))
	for _, proposal := range unresolved {
		n := s.proposalRetries[proposal.SigningRoot]
		if n >= maxProposalRetries {
			delete(s.proposalRetries, proposal.SigningRoot)
			header := proposal.SignedBeaconBlockHeader.Header
			log.WithField("slot", header.Slot).WithField("proposerIndex", header.ProposerIndex).
				Warn("Giving up on a conflicting block header whose signature could not be verified")
			continue
		}
		s.proposalRetries[proposal.SigningRoot] = n + 1
		keep = append(keep, proposal)
	}
	s.blksQueue.extend(keep)
}

// forgetProposalRetries drops the retry bookkeeping of settled proposals.
func (s *Service) forgetProposalRetries(proposals []*slashertypes.SignedBlockHeaderWrapper) {
	for _, proposal := range proposals {
		delete(s.proposalRetries, proposal.SigningRoot)
	}
}
