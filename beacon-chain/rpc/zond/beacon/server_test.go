package beacon

import zondpbservice "github.com/theQRL/qrysm/proto/zond/service"

var _ zondpbservice.BeaconChainServer = (*Server)(nil)
