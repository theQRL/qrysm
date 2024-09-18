package node

import (
	zondpbservice "github.com/theQRL/qrysm/proto/zond/service"
)

var _ zondpbservice.BeaconNodeServer = (*Server)(nil)
