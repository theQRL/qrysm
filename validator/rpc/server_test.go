package rpc

import (
	pb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1/validator-client"
)

var _ pb.AuthServer = (*Server)(nil)
