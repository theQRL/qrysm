package beacon

import ethpbservice "github.com/cyyber/qrysm/v4/proto/eth/service"

var _ ethpbservice.BeaconChainServer = (*Server)(nil)
