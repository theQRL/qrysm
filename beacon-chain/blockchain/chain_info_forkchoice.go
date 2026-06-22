package blockchain

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/state"
	consensus_blocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
)

// CachedHeadRoot returns the corresponding value from Forkchoice
func (s *Service) CachedHeadRoot() [32]byte {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.CachedHeadRoot()
}

// GetProposerHead returns the corresponding value from forkchoice
func (s *Service) GetProposerHead() [32]byte {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.GetProposerHead()
}

// SetForkChoiceGenesisTime sets the genesis time in Forkchoice
func (s *Service) SetForkChoiceGenesisTime(timestamp uint64) {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	s.cfg.ForkChoiceStore.SetGenesisTime(timestamp)
}

// HighestReceivedBlockSlot returns the corresponding value from forkchoice
func (s *Service) HighestReceivedBlockSlot() primitives.Slot {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.HighestReceivedBlockSlot()
}

// ReceivedBlocksLastEpoch returns the corresponding value from forkchoice
func (s *Service) ReceivedBlocksLastEpoch() (uint64, error) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.ReceivedBlocksLastEpoch()
}

// InsertNode is a wrapper for node insertion which is self locked
func (s *Service) InsertNode(ctx context.Context, st state.BeaconState, block consensus_blocks.ROBlock) error {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	return s.cfg.ForkChoiceStore.InsertNode(ctx, st, block)
}

// ForkChoiceDump returns the corresponding value from forkchoice
func (s *Service) ForkChoiceDump(ctx context.Context) (*qrlpb.ForkChoiceDump, error) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.ForkChoiceDump(ctx)
}

// NewSlot returns the corresponding value from forkchoice
func (s *Service) NewSlot(ctx context.Context, slot primitives.Slot) error {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	return s.cfg.ForkChoiceStore.NewSlot(ctx, slot)
}

// ProposerBoost wraps the corresponding method from forkchoice
func (s *Service) ProposerBoost() [32]byte {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	return s.cfg.ForkChoiceStore.ProposerBoost()
}

// ChainHeads returns all possible chain heads (leaves of fork choice tree).
// Heads roots and heads slots are returned.
func (s *Service) ChainHeads() ([][32]byte, []primitives.Slot) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.Tips()
}

// UnrealizedJustifiedPayloadBlockHash returns unrealized justified payload block hash from forkchoice.
func (s *Service) UnrealizedJustifiedPayloadBlockHash() [32]byte {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.UnrealizedJustifiedPayloadBlockHash()
}

// FinalizedBlockHash returns finalized payload block hash from forkchoice.
func (s *Service) FinalizedBlockHash() [32]byte {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.FinalizedPayloadBlockHash()
}

// hashForGenesisBlock returns the execution block hash recorded in the genesis
// state's LatestExecutionPayloadHeader, but only when `root` is in fact the
// genesis block root. Used as a defensive fallback when an FCU would otherwise
// be sent with a zero/empty HeadBlockHash at genesis.
//
// Returns errNotGenesisRoot if `root` is not the genesis block root, so callers
// can distinguish "this isn't a genesis FCU" from real DB / state errors.
func (s *Service) hashForGenesisBlock(ctx context.Context, root [32]byte) ([]byte, error) {
	genRoot, err := s.cfg.BeaconDB.GenesisBlockRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get genesis block root")
	}
	if root != genRoot {
		return nil, errNotGenesisRoot
	}
	st, err := s.cfg.BeaconDB.GenesisState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get genesis state")
	}
	header, err := st.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, errors.Wrap(err, "could not get latest execution payload header")
	}
	return bytesutil.SafeCopyBytes(header.BlockHash()), nil
}
