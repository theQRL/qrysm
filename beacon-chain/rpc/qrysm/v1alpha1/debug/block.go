package debug

import (
	"context"

	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	pbrpc "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetBlock in an ssz-encoded format by block root.
func (ds *Server) GetBlock(
	ctx context.Context,
	req *pbrpc.BlockRequestByRoot,
) (*pbrpc.SSZResponse, error) {
	root := bytesutil.ToBytes32(req.BlockRoot)
	signedBlock, err := ds.BeaconDB.Block(ctx, root)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not retrieve block by root: %v", err)
	}
	if signedBlock == nil || signedBlock.IsNil() {
		return &pbrpc.SSZResponse{Encoded: make([]byte, 0)}, nil
	}
	encoded, err := signedBlock.MarshalSSZ()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not marshal block: %v", err)
	}
	return &pbrpc.SSZResponse{
		Encoded: encoded,
	}, nil
}
