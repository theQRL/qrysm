package validator_helpers

import (
	"time"

	"google.golang.org/grpc"
)

// Use an interface with a private dummy function to force all other packages to call NewNodeConnection
type NodeConnection interface {
	GetGrpcClientConn() grpc.ClientConnInterface
	GetBeaconApiUrl() string
	GetBeaconApiTimeout() time.Duration
	Close() error
	dummy()
}

type nodeConnection struct {
	grpcClientConn   grpc.ClientConnInterface
	beaconApiUrl     string
	beaconApiTimeout time.Duration
	closeFn          func() error
}

func (c *nodeConnection) GetGrpcClientConn() grpc.ClientConnInterface {
	return c.grpcClientConn
}

func (c *nodeConnection) GetBeaconApiUrl() string {
	return c.beaconApiUrl
}

func (c *nodeConnection) GetBeaconApiTimeout() time.Duration {
	return c.beaconApiTimeout
}

func (c *nodeConnection) Close() error {
	if c.closeFn == nil {
		return nil
	}
	return c.closeFn()
}

func (*nodeConnection) dummy() {}

func NewNodeConnection(grpcConn grpc.ClientConnInterface, beaconApiUrl string, beaconApiTimeout time.Duration, closeFn func() error) NodeConnection {
	conn := &nodeConnection{}
	conn.grpcClientConn = grpcConn
	conn.beaconApiUrl = beaconApiUrl
	conn.beaconApiTimeout = beaconApiTimeout
	conn.closeFn = closeFn
	return conn
}
