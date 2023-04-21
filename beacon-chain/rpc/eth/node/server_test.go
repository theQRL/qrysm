package node

import (
	ethpbservice "github.com/cyyber/qrysm/v4/proto/eth/service"
)

var _ ethpbservice.BeaconNodeServer = (*Server)(nil)
