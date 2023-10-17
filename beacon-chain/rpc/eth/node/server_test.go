package node

import (
	ethpbservice "github.com/theQRL/qrysm/v4/proto/eth/service"
)

var _ ethpbservice.BeaconNodeServer = (*Server)(nil)
