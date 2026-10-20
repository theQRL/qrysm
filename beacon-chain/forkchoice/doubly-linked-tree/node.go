package doublylinkedtree

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	"github.com/theQRL/qrysm/time/slots"
)

// ProcessAttestationsThreshold returns the number of seconds after which we
// process attestations for the current slot. It is derived from the slot
// duration as SecondsPerSlot * 5 / 6, matching the upstream Ethereum value of
// 10 for a 12-second slot and yielding 50 for qrysm's 60-second slot.
const ProcessAttestationsThreshold = 50

// applyWeightChanges recomputes the weight of the node passed as an argument and all of its descendants,
// using the current balance stored in each node.
func (n *Node) applyWeightChanges(ctx context.Context) error {
	// Recursively calling the children to sum their weights.
	childrenWeight := uint64(0)
	for _, child := range n.children {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := child.applyWeightChanges(ctx); err != nil {
			return err
		}
		childrenWeight += child.weight
	}
	if n.root == params.BeaconConfig().ZeroHash {
		return nil
	}
	n.weight = n.balance + childrenWeight
	return nil
}

// updateBestDescendant updates the best descendant of this node and its
// children. Only branches leading to a viable tip survive filter_block_tree.
// If none survive, head selection falls back to the justified checkpoint.
func (n *Node) updateBestDescendant(ctx context.Context, justifiedEpoch, finalizedEpoch, currentEpoch primitives.Epoch) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(n.children) == 0 {
		n.bestDescendant = nil
		return nil
	}
	for _, child := range n.children {
		if child == nil {
			return errors.Wrap(ErrNilNode, "could not update best descendant")
		}
		if err := child.updateBestDescendant(ctx, justifiedEpoch, finalizedEpoch, currentEpoch); err != nil {
			return err
		}
	}
	bestChild := n.bestChild(justifiedEpoch, currentEpoch)
	switch {
	case bestChild == nil:
		n.bestDescendant = nil
	case bestChild.bestDescendant == nil:
		n.bestDescendant = bestChild
	default:
		n.bestDescendant = bestChild.bestDescendant
	}
	return nil
}

// bestChild returns the heaviest child leading to a viable tip, breaking ties
// by the larger root, or nil when no child does.
func (n *Node) bestChild(justifiedEpoch, currentEpoch primitives.Epoch) *Node {
	var best *Node
	for _, child := range n.children {
		if !child.leadsToViableTip(justifiedEpoch, currentEpoch) {
			continue
		}
		if best == nil || child.weight > best.weight || (child.weight == best.weight && bytes.Compare(child.root[:], best.root[:]) > 0) {
			best = child
		}
	}
	return best
}

// viableForHead checks the node's voting source against the justified and
// current epochs.
func (n *Node) viableForHead(justifiedEpoch, currentEpoch primitives.Epoch) bool {
	if justifiedEpoch == 0 {
		return true
	}
	votingSource := n.justifiedEpoch
	// A block can arrive before the epoch tick has realized older nodes'
	// checkpoints. Select their pulled-up voting source directly.
	if slots.ToEpoch(n.slot) < currentEpoch {
		votingSource = n.unrealizedJustifiedEpoch
	}
	return votingSource == justifiedEpoch || votingSource+2 >= currentEpoch
}

// leadsToViableTip reports whether the node is in the spec's filtered block
// tree: a leaf that is viable for head, or a block whose best descendant is
// such a leaf. A block with children is admitted only through a viable tip.
func (n *Node) leadsToViableTip(justifiedEpoch, currentEpoch primitives.Epoch) bool {
	tip := n.bestDescendant
	if tip == nil {
		tip = n
	}
	return len(tip.children) == 0 && tip.viableForHead(justifiedEpoch, currentEpoch)
}

// setNodeAndParentValidated sets the current node and all the ancestors as validated (i.e. non-optimistic).
func (n *Node) setNodeAndParentValidated(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if !n.optimistic {
		return nil
	}

	// Validate ancestors first so cancellation cannot leave a validated child
	// that prevents a later retry from reaching its optimistic parent.
	if n.parent != nil {
		if err := n.parent.setNodeAndParentValidated(ctx); err != nil {
			return err
		}
	}
	n.optimistic = false
	return nil
}

// arrivedEarly returns whether this node was inserted before the first
// threshold to orphan a block.
// Note that genesisTime has seconds granularity, therefore we use a strict
// inequality < here. For example a block that arrives at exactly the voting
// window boundary will still be considered early.
func (n *Node) arrivedEarly(genesisTime uint64) (bool, error) {
	// The tree root node (genesis or the finalized checkpoint on
	// checkpoint-sync) carries an insertion timestamp sourced from time.Now()
	// at process startup, not actual network arrival time. It can never be a
	// reorg target anyway, so treat it as always having arrived early.
	if n.parent == nil {
		return true, nil
	}
	secs, err := slots.SecondsSinceSlotStart(n.slot, genesisTime, n.timestamp)
	votingWindow := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
	return secs < votingWindow, err
}

// arrivedAfterOrphanCheck returns whether this block was inserted after the
// intermediate checkpoint to check for candidate of being orphaned.
// Note that genesisTime has seconds granularity, therefore we use an
// inequality >= here. For example a block that arrives 10.00001 seconds into the
// slot will have secs = 10 below.
func (n *Node) arrivedAfterOrphanCheck(genesisTime uint64) (bool, error) {
	// See arrivedEarly: the tree root's timestamp is not meaningful here.
	if n.parent == nil {
		return false, nil
	}
	secs, err := slots.SecondsSinceSlotStart(n.slot, genesisTime, n.timestamp)
	return secs >= ProcessAttestationsThreshold, err
}

// nodeTreeDump appends to the given list all the nodes descending from this one
func (n *Node) nodeTreeDump(ctx context.Context, nodes []*qrlpb.ForkChoiceNode) ([]*qrlpb.ForkChoiceNode, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var parentRoot [32]byte
	if n.parent != nil {
		parentRoot = n.parent.root
	}
	thisNode := &qrlpb.ForkChoiceNode{
		Slot:                     n.slot,
		BlockRoot:                n.root[:],
		ParentRoot:               parentRoot[:],
		JustifiedEpoch:           n.justifiedEpoch,
		FinalizedEpoch:           n.finalizedEpoch,
		UnrealizedJustifiedEpoch: n.unrealizedJustifiedEpoch,
		UnrealizedFinalizedEpoch: n.unrealizedFinalizedEpoch,
		Balance:                  n.balance,
		Weight:                   n.weight,
		ExecutionOptimistic:      n.optimistic,
		ExecutionBlockHash:       n.payloadHash[:],
		Timestamp:                n.timestamp,
	}
	if n.optimistic {
		thisNode.Validity = qrlpb.ForkChoiceNodeValidity_OPTIMISTIC
	} else {
		thisNode.Validity = qrlpb.ForkChoiceNodeValidity_VALID
	}

	nodes = append(nodes, thisNode)
	var err error
	for _, child := range n.children {
		nodes, err = child.nodeTreeDump(ctx, nodes)
		if err != nil {
			return nil, err
		}
	}
	return nodes, nil
}
