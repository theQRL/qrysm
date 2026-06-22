package node

import (
	qrlpbservice "github.com/theQRL/qrysm/proto/qrl/service"
)

var _ qrlpbservice.BeaconNodeServer = (*Server)(nil)
