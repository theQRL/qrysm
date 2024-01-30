package beacon

import zondpbservice "github.com/theQRL/qrysm/v4/proto/zond/service"

var _ zondpbservice.BeaconChainServer = (*Server)(nil)
