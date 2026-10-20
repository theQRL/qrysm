package doublylinkedtree

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	consensus_blocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/time/slots"
	"go.opencensus.io/trace"
)

// head starts from justified root and then follows the best descendant links
// to find the best block for head.
func (s *Store) head(ctx context.Context) ([32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.head")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return [32]byte{}, err
	}

	// JustifiedRoot has to be known
	justifiedNode, ok := s.nodeByRoot[s.justifiedCheckpoint.Root]
	if !ok || justifiedNode == nil {
		// If the justifiedCheckpoint is from genesis, then the root is
		// zeroHash. In this case it should be the root of forkchoice
		// tree.
		if s.justifiedCheckpoint.Epoch == params.BeaconConfig().GenesisEpoch {
			justifiedNode = s.treeRootNode
		} else {
			return [32]byte{}, errors.WithMessage(errUnknownJustifiedRoot, fmt.Sprintf("%#x", s.justifiedCheckpoint.Root))
		}
	}

	// The justified checkpoint is the default head when the filtered tree
	// has no eligible descendants.
	bestDescendant := justifiedNode.bestDescendant
	if bestDescendant == nil {
		bestDescendant = justifiedNode
	}
	currentEpoch := slots.EpochsSinceGenesis(time.Unix(int64(s.genesisTime), 0))
	// A checkpoint is justified by later blocks, so its own voting source
	// can be stale. It must still be returned when no eligible tip exists.
	if bestDescendant != justifiedNode && !bestDescendant.viableForHead(s.justifiedCheckpoint.Epoch, currentEpoch) {
		s.allTipsAreInvalid = true
		return [32]byte{}, fmt.Errorf("head at slot %d with weight %d is not eligible, finalizedEpoch, justified Epoch %d, %d != %d, %d",
			bestDescendant.slot, bestDescendant.weight/10e9, bestDescendant.finalizedEpoch, bestDescendant.justifiedEpoch, s.finalizedCheckpoint.Epoch, s.justifiedCheckpoint.Epoch)
	}
	// Returning the checkpoint does not establish that a viable branch exists.
	// If execution invalidation emptied the filtered tree, remain optimistic
	// until a viable tip becomes available again.
	if justifiedNode.leadsToViableTip(s.justifiedCheckpoint.Epoch, currentEpoch) {
		s.allTipsAreInvalid = false
	}

	// Update metrics.
	if bestDescendant != s.headNode {
		headChangesCount.Inc()
		headSlotNumber.Set(float64(bestDescendant.slot))
		s.headNode = bestDescendant
	}

	return bestDescendant.root, nil
}

// insert registers a new block node to the fork choice store's node list.
// It then updates the new node's parent with best child and descendant node.
func (s *Store) insert(ctx context.Context,
	roblock consensus_blocks.ROBlock,
	justifiedEpoch, finalizedEpoch primitives.Epoch) (*Node, error) {
	ctx, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.insert")
	defer span.End()

	root := roblock.Root()
	block := roblock.Block()
	slot := block.Slot()
	parentRoot := block.ParentRoot()
	var payloadHash [32]byte
	execution, err := block.Body().Execution()
	if err != nil {
		return nil, err
	}
	copy(payloadHash[:], execution.BlockHash())

	// Return if the block has been inserted into Store before.
	if n, ok := s.nodeByRoot[root]; ok {
		return n, nil
	}

	parent := s.nodeByRoot[parentRoot]

	n := &Node{
		slot:                     slot,
		root:                     root,
		parent:                   parent,
		justifiedEpoch:           justifiedEpoch,
		unrealizedJustifiedEpoch: justifiedEpoch,
		finalizedEpoch:           finalizedEpoch,
		unrealizedFinalizedEpoch: finalizedEpoch,
		optimistic:               true,
		payloadHash:              payloadHash,
		timestamp:                uint64(time.Now().Unix()),
	}

	s.nodeByPayload[payloadHash] = n
	s.nodeByRoot[root] = n
	if parent == nil {
		if s.treeRootNode == nil {
			s.treeRootNode = n
			s.headNode = n
			s.highestReceivedNode = n
		} else {
			delete(s.nodeByRoot, root)
			delete(s.nodeByPayload, payloadHash)
			return n, errInvalidParentRoot
		}
	} else {
		parent.children = append(parent.children, n)
		// Apply proposer boost
		timeNow := uint64(time.Now().Unix())
		if timeNow < s.genesisTime {
			return n, nil
		}
		secondsIntoSlot := (timeNow - s.genesisTime) % params.BeaconConfig().SecondsPerSlot
		currentSlot := slots.CurrentSlot(s.genesisTime)
		boostThreshold := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
		// Insertion can acquire the store lock before the slot ticker does.
		// Expire the previous slot's boost before considering this block.
		s.expireProposerBoost(currentSlot)
		isFirstBlock := s.proposerBoostRoot == [32]byte{}
		if currentSlot == slot && secondsIntoSlot < boostThreshold && isFirstBlock {
			s.proposerBoostRoot = root
		}

		// Update best descendants
		jEpoch := s.justifiedCheckpoint.Epoch
		fEpoch := s.finalizedCheckpoint.Epoch
		if err := s.treeRootNode.updateBestDescendant(ctx, jEpoch, fEpoch, slots.ToEpoch(currentSlot)); err != nil {
			_, remErr := s.removeNode(context.WithoutCancel(ctx), n)
			if remErr != nil {
				log.WithError(remErr).Error("could not remove node")
			}
			return nil, errors.Wrap(err, "could not update best descendants")
		}
	}
	// Update metrics.
	processedBlockCount.Inc()
	nodeCount.Set(float64(len(s.nodeByRoot)))

	// Only update received block slot if it's within epoch from current time.
	if slot+params.BeaconConfig().SlotsPerEpoch > slots.CurrentSlot(s.genesisTime) {
		s.receivedBlocksLastEpoch[slot%params.BeaconConfig().SlotsPerEpoch] = slot
	}
	// Update highest slot tracking.
	if slot > s.highestReceivedNode.slot {
		s.highestReceivedNode = n
	}

	return n, nil
}

// prunableNodes collects nodes outside the finalized subtree without changing
// links or indexes. Cancellation must leave the entire tree reachable.
func prunableNodes(ctx context.Context, node, finalizedNode *Node, nodes []*Node) ([]*Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if node == finalizedNode {
		return nodes, nil
	}
	var err error
	for _, child := range node.children {
		if nodes, err = prunableNodes(ctx, child, finalizedNode, nodes); err != nil {
			return nil, err
		}
	}
	return append(nodes, node), nil
}

type pruningPlan struct {
	finalized *Node
	removed   []*Node
	children  []*Node
}

// preparePrune performs every fallible step before checkpoints or tree links
// are changed. The caller holds the forkchoice lock through applyPrune.
func (s *Store) preparePrune(ctx context.Context, checkpoint *forkchoicetypes.Checkpoint) (*pruningPlan, error) {
	ctx, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.Prune")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	finalizedNode := s.nodeByRoot[checkpoint.Root]
	// Before the first finalization, checkpoints use the zero-root genesis
	// alias while the tree indexes genesis by its actual block root.
	if finalizedNode == nil && checkpoint.Epoch == params.BeaconConfig().GenesisEpoch &&
		checkpoint.Root == params.BeaconConfig().ZeroHash && s.treeRootNode != nil && s.treeRootNode.slot == 0 {
		finalizedNode = s.treeRootNode
	}
	if finalizedNode == nil {
		return nil, errors.WithMessage(errUnknownFinalizedRoot, fmt.Sprintf("%#x", checkpoint.Root))
	}
	checkpointMaxSlot, err := slots.EpochStart(checkpoint.Epoch)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute epoch start")
	}
	plan := &pruningPlan{finalized: finalizedNode}
	if finalizedNode.parent != nil {
		plan.removed, err = prunableNodes(ctx, s.treeRootNode, finalizedNode, nil)
		if err != nil {
			return nil, err
		}
	}

	// The finalized epoch can advance at the same root when slots were
	// skipped. Build a separate slice so interrupted collection cannot
	// overwrite child links that are still in use.
	for _, child := range finalizedNode.children {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if finalizedNode.slot != checkpointMaxSlot && child != nil && child.slot <= checkpointMaxSlot {
			plan.removed, err = child.subtreeNodes(ctx, plan.removed)
			if err != nil {
				return nil, errors.Wrap(err, "could not prune incompatible finalized child")
			}
			continue
		}
		plan.children = append(plan.children, child)
	}
	return plan, nil
}

// applyPrune completes a prepared prune without further cancellation points.
func (s *Store) applyPrune(plan *pruningPlan) {
	for _, node := range plan.removed {
		node.children = nil
		delete(s.nodeByRoot, node.root)
		delete(s.nodeByPayload, node.payloadHash)
	}
	if plan.finalized.parent != nil {
		prunedCount.Inc()
	}
	plan.finalized.parent = nil
	plan.finalized.children = plan.children
	s.treeRootNode = plan.finalized
	s.finalizedPayloadBlockHash = plan.finalized.payloadHash
}

// prune removes nodes that compete with the finalized checkpoint.
func (s *Store) prune(ctx context.Context) error {
	plan, err := s.preparePrune(ctx, s.finalizedCheckpoint)
	if err != nil {
		return err
	}
	s.applyPrune(plan)
	return nil
}

// tips returns a list of possible heads from fork choice store, it returns the
// roots and the slots of the leaf nodes.
func (s *Store) tips() ([][32]byte, []primitives.Slot) {
	var roots [][32]byte
	var slots []primitives.Slot

	for root, node := range s.nodeByRoot {
		if len(node.children) == 0 {
			roots = append(roots, root)
			slots = append(slots, node.slot)
		}
	}
	return roots, slots
}

func (f *ForkChoice) HighestReceivedBlockRoot() [32]byte {
	if f.store.highestReceivedNode == nil {
		return [32]byte{}
	}
	return f.store.highestReceivedNode.root
}

// HighestReceivedBlockSlot returns the highest slot received by the forkchoice
func (f *ForkChoice) HighestReceivedBlockSlot() primitives.Slot {
	if f.store.highestReceivedNode == nil {
		return 0
	}
	return f.store.highestReceivedNode.slot
}

// ReceivedBlocksLastEpoch returns the number of blocks received in the last epoch
func (f *ForkChoice) ReceivedBlocksLastEpoch() (uint64, error) {
	count := uint64(0)
	lowerBound := slots.CurrentSlot(f.store.genesisTime)
	var err error
	if lowerBound > fieldparams.SlotsPerEpoch {
		lowerBound, err = lowerBound.SafeSub(fieldparams.SlotsPerEpoch)
		if err != nil {
			return 0, err
		}
	}

	for _, s := range f.store.receivedBlocksLastEpoch {
		if s != 0 && lowerBound <= s {
			count++
		}
	}
	return count, nil
}
