package state_native

import (
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// LatestExecutionPayloadHeader of the beacon state.
func (b *BeaconState) LatestExecutionPayloadHeader() (interfaces.ExecutionData, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return blocks.WrappedExecutionPayloadHeaderZond(b.latestExecutionPayloadHeaderZondVal(), 0)
}

// latestExecutionPayloadHeaderZondVal of the beacon state.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) latestExecutionPayloadHeaderZondVal() *enginev1.ExecutionPayloadHeaderZond {
	return qrysmpb.CopyExecutionPayloadHeaderZond(b.latestExecutionPayloadHeaderZond)
}
