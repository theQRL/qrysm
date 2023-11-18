package state_native

import (
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
)

// LatestExecutionPayloadHeader of the beacon state.
func (b *BeaconState) LatestExecutionPayloadHeader() (interfaces.ExecutionData, error) {
	if b.version < version.Bellatrix {
		return nil, errNotSupported("LatestExecutionPayloadHeader", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.version == version.Bellatrix {
		return blocks.WrappedExecutionPayloadHeader(b.latestExecutionPayloadHeaderVal())
	}

	if b.version == version.Capella {
		return blocks.WrappedExecutionPayloadHeaderCapella(b.latestExecutionPayloadHeaderCapellaVal(), 0)
	}

	return blocks.WrappedExecutionPayloadHeaderDeneb(b.latestExecutionPayloadHeaderDenebVal(), 0)
}

// latestExecutionPayloadHeaderVal of the beacon state.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) latestExecutionPayloadHeaderVal() *enginev1.ExecutionPayloadHeader {
	return zondpb.CopyExecutionPayloadHeader(b.latestExecutionPayloadHeader)
}

// latestExecutionPayloadHeaderCapellaVal of the beacon state.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) latestExecutionPayloadHeaderCapellaVal() *enginev1.ExecutionPayloadHeaderCapella {
	return zondpb.CopyExecutionPayloadHeaderCapella(b.latestExecutionPayloadHeaderCapella)
}

func (b *BeaconState) latestExecutionPayloadHeaderDenebVal() *enginev1.ExecutionPayloadHeaderDeneb {
	return zondpb.CopyExecutionPayloadHeaderDeneb(b.latestExecutionPayloadHeaderDeneb)
}
