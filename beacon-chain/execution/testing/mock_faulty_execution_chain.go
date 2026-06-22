package testing

import (
	"github.com/theQRL/qrysm/async/event"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// FaultyExecutionChain defines an incorrectly functioning execution chain service.
type FaultyExecutionChain struct {
	ChainFeed      *event.Feed
	HashesByHeight map[int][]byte
}

// ChainStartExecutionData --
func (*FaultyExecutionChain) ChainStartExecutionData() *qrysmpb.ExecutionData {
	return &qrysmpb.ExecutionData{}
}
