package beacon

import qrlpbservice "github.com/theQRL/qrysm/proto/qrl/service"

var _ qrlpbservice.BeaconChainServer = (*Server)(nil)
