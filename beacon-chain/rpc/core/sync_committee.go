package core

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	p2ptypes "github.com/theQRL/qrysm/beacon-chain/p2p/types"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// validateSyncMessage checks RPC submissions before they can be broadcast or
// enter the pool. Proposers rely on pool admission to verify sync signatures.
func (s *Service) validateSyncMessage(ctx context.Context, msg *qrysmpb.SyncCommitteeMessage) ([]primitives.CommitteeIndex, *RpcError) {
	if msg == nil {
		return nil, &RpcError{Reason: BadRequest, Err: errors.New("sync committee message can't be nil")}
	}
	if len(msg.BlockRoot) != fieldparams.RootLength || len(msg.Signature) != fieldparams.MLDSA87SignatureLength {
		return nil, &RpcError{Reason: BadRequest, Err: errors.New("invalid sync committee message root or signature length")}
	}
	// Check time before requesting head data, which may advance the head state
	// to the supplied slot on a cache miss.
	if err := altair.ValidateSyncMessageTime(msg.Slot, s.GenesisTimeFetcher.GenesisTime(), params.BeaconNetworkConfig().MaximumGossipClockDisparity); err != nil {
		return nil, &RpcError{Reason: BadRequest, Err: err}
	}
	indices, err := s.HeadFetcher.HeadSyncCommitteeIndices(ctx, msg.ValidatorIndex, msg.Slot)
	if err != nil {
		return nil, &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not get head sync committee indices")}
	}
	if len(indices) == 0 {
		return nil, &RpcError{Reason: BadRequest, Err: errors.New("validator is not in the sync committee")}
	}
	pubkey, err := s.HeadFetcher.HeadValidatorIndexToPublicKey(ctx, msg.ValidatorIndex)
	if err != nil {
		return nil, &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not get sync committee public key")}
	}
	domain, err := s.HeadFetcher.HeadSyncCommitteeDomain(ctx, msg.Slot)
	if err != nil {
		return nil, &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not get sync committee domain")}
	}
	root := p2ptypes.SSZBytes(msg.BlockRoot)
	if err := signing.VerifySigningRoot(&root, pubkey[:], msg.Signature, domain); err != nil {
		return nil, &RpcError{Reason: BadRequest, Err: errors.Wrap(err, "invalid sync committee signature")}
	}
	return indices, nil
}

// validateSyncContribution verifies both aggregator proofs and every participant
// signature, in aggregation-bit order, before RPC pool admission or broadcast.
func (s *Service) validateSyncContribution(ctx context.Context, req *qrysmpb.SignedContributionAndProof) *RpcError {
	if err := altair.ValidateNilSyncContribution(req); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	c := req.Message.Contribution
	cfg := params.BeaconConfig()
	if c.SubcommitteeIndex >= cfg.SyncCommitteeSubnetCount {
		return &RpcError{Reason: BadRequest, Err: errors.New("invalid sync subcommittee index")}
	}
	if len(c.BlockRoot) != fieldparams.RootLength || len(c.AggregationBits) != len(qrysmpb.NewSyncCommitteeAggregationBits()) {
		return &RpcError{Reason: BadRequest, Err: errors.New("invalid sync contribution root or aggregation bits length")}
	}
	if c.AggregationBits.Count() == 0 || c.AggregationBits.Count() != uint64(len(c.Signatures)) {
		return &RpcError{Reason: BadRequest, Err: errors.New("sync contribution signatures must match the nonempty aggregation bits")}
	}
	if len(req.Signature) != fieldparams.MLDSA87SignatureLength || len(req.Message.SelectionProof) != fieldparams.MLDSA87SignatureLength {
		return &RpcError{Reason: BadRequest, Err: errors.New("invalid sync contribution proof signature length")}
	}
	for _, sig := range c.Signatures {
		if len(sig) != fieldparams.MLDSA87SignatureLength {
			return &RpcError{Reason: BadRequest, Err: errors.New("invalid sync contribution participant signature length")}
		}
	}
	if err := altair.ValidateSyncMessageTime(c.Slot, s.GenesisTimeFetcher.GenesisTime(), params.BeaconNetworkConfig().MaximumGossipClockDisparity); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	indices, err := s.HeadFetcher.HeadSyncCommitteeIndices(ctx, req.Message.AggregatorIndex, c.Slot)
	if err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not get aggregator sync committee indices")}
	}
	subcommitteeSize := cfg.SyncCommitteeSize / cfg.SyncCommitteeSubnetCount
	member := false
	for _, index := range indices {
		if uint64(index)/subcommitteeSize == c.SubcommitteeIndex {
			member = true
			break
		}
	}
	if !member {
		return &RpcError{Reason: BadRequest, Err: errors.New("aggregator is not in the sync subcommittee")}
	}
	seed, err := s.HeadFetcher.HeadAggregatorSelectionSeed(ctx, c.Slot)
	if err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not get aggregator selection seed")}
	}
	selected, err := altair.IsSyncCommitteeAggregator(seed[:], c.Slot, c.SubcommitteeIndex, req.Message.AggregatorIndex)
	if err != nil || !selected {
		return &RpcError{Reason: BadRequest, Err: errors.New("validator is not a sync committee aggregator")}
	}
	pubkey, err := s.HeadFetcher.HeadValidatorIndexToPublicKey(ctx, req.Message.AggregatorIndex)
	if err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not get aggregator public key")}
	}
	selectionDomain, err := s.HeadFetcher.HeadSyncSelectionProofDomain(ctx, c.Slot)
	if err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not get sync selection proof domain")}
	}
	selection := &qrysmpb.SyncAggregatorSelectionData{Slot: c.Slot, SubcommitteeIndex: c.SubcommitteeIndex}
	if err := signing.VerifySigningRoot(selection, pubkey[:], req.Message.SelectionProof, selectionDomain); err != nil {
		return &RpcError{Reason: BadRequest, Err: errors.Wrap(err, "invalid sync selection proof")}
	}
	contributionDomain, err := s.HeadFetcher.HeadSyncContributionProofDomain(ctx, c.Slot)
	if err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not get sync contribution proof domain")}
	}
	if err := signing.VerifySigningRoot(req.Message, pubkey[:], req.Signature, contributionDomain); err != nil {
		return &RpcError{Reason: BadRequest, Err: errors.Wrap(err, "invalid sync contribution signature")}
	}
	committee, err := s.HeadFetcher.HeadSyncCommitteePubKeys(ctx, c.Slot, primitives.CommitteeIndex(c.SubcommitteeIndex))
	if err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not get sync subcommittee public keys")}
	}
	if uint64(len(committee)) != c.AggregationBits.Len() {
		return &RpcError{Reason: Internal, Err: errors.New("sync subcommittee size does not match aggregation bits")}
	}
	pubkeys := make([]ml_dsa_87.PublicKey, 0, len(c.Signatures))
	for i, pk := range committee {
		if !c.AggregationBits.BitAt(uint64(i)) {
			continue
		}
		pubkey, err := ml_dsa_87.PublicKeyFromBytes(pk)
		if err != nil {
			return &RpcError{Reason: Internal, Err: errors.Wrap(err, "invalid sync subcommittee public key")}
		}
		pubkeys = append(pubkeys, pubkey)
	}
	domain, err := s.HeadFetcher.HeadSyncCommitteeDomain(ctx, c.Slot)
	if err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not get sync committee domain")}
	}
	root := p2ptypes.SSZBytes(c.BlockRoot)
	signingRoot, err := signing.ComputeSigningRoot(&root, domain)
	if err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not compute sync contribution signing root")}
	}
	valid, err := ml_dsa_87.VerifyMultipleSignatures([][][]byte{c.Signatures}, [][32]byte{signingRoot}, [][]ml_dsa_87.PublicKey{pubkeys})
	if err != nil {
		return &RpcError{Reason: BadRequest, Err: errors.Wrap(err, "invalid sync contribution participant signatures")}
	}
	if !valid {
		return &RpcError{Reason: BadRequest, Err: errors.New("invalid sync contribution participant signatures")}
	}
	return nil
}
