package state_native

import (
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/state/state-native/types"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	_ "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// SetLatestExecutionPayloadHeader for the beacon state.
func (b *BeaconState) SetLatestExecutionPayloadHeader(val interfaces.ExecutionData) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	switch header := val.Proto().(type) {
	case *enginev1.ExecutionPayloadZond:
		latest, err := consensusblocks.PayloadToHeaderZond(val)
		if err != nil {
			return errors.Wrap(err, "could not convert payload to header")
		}
		b.latestExecutionPayloadHeaderZond = latest
		b.markFieldAsDirty(types.LatestExecutionPayloadHeaderZond)
		return nil
	case *enginev1.ExecutionPayloadHeaderZond:
		b.latestExecutionPayloadHeaderZond = header
		b.markFieldAsDirty(types.LatestExecutionPayloadHeaderZond)
		return nil
	default:
		return errors.New("value must be an execution payload header")
	}
}
