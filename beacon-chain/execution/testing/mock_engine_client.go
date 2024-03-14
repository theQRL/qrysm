package testing

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	payloadattribute "github.com/theQRL/qrysm/v4/consensus-types/payload-attribute"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	"github.com/theQRL/qrysm/v4/math"
	pb "github.com/theQRL/qrysm/v4/proto/engine/v1"
)

// EngineClient --
type EngineClient struct {
	NewPayloadResp              []byte
	PayloadIDBytes              *pb.PayloadIDBytes
	ForkChoiceUpdatedResp       []byte
	ExecutionPayloadCapella     *pb.ExecutionPayloadCapella
	ExecutionBlock              *pb.ExecutionBlock
	Err                         error
	ErrLatestExecBlock          error
	ErrExecBlockByHash          error
	ErrForkchoiceUpdated        error
	ErrNewPayload               error
	ErrGetPayload               error
	ExecutionPayloadByBlockHash map[[32]byte]*pb.ExecutionPayloadCapella
	BlockByHashMap              map[[32]byte]*pb.ExecutionBlock
	NumReconstructedPayloads    uint64
	TerminalBlockHash           []byte
	TerminalBlockHashExists     bool
	BuilderOverride             bool
	OverrideValidHash           [32]byte
	BlockValue                  uint64
}

// NewPayload --
func (e *EngineClient) NewPayload(_ context.Context, _ interfaces.ExecutionData, _ []common.Hash, _ *common.Hash) ([]byte, error) {
	return e.NewPayloadResp, e.ErrNewPayload
}

// ForkchoiceUpdated --
func (e *EngineClient) ForkchoiceUpdated(
	_ context.Context, fcs *pb.ForkchoiceState, _ payloadattribute.Attributer,
) (*pb.PayloadIDBytes, []byte, error) {
	if e.OverrideValidHash != [32]byte{} && bytesutil.ToBytes32(fcs.HeadBlockHash) == e.OverrideValidHash {
		return e.PayloadIDBytes, e.ForkChoiceUpdatedResp, nil
	}
	return e.PayloadIDBytes, e.ForkChoiceUpdatedResp, e.ErrForkchoiceUpdated
}

// GetPayload --
func (e *EngineClient) GetPayload(_ context.Context, _ [8]byte, s primitives.Slot) (interfaces.ExecutionData, bool, error) {
	ed, err := blocks.WrappedExecutionPayloadCapella(e.ExecutionPayloadCapella, math.Gwei(e.BlockValue))
	if err != nil {
		return nil, false, err
	}
	return ed, e.BuilderOverride, nil
}

// ExchangeTransitionConfiguration --
func (e *EngineClient) ExchangeTransitionConfiguration(_ context.Context, _ *pb.TransitionConfiguration) error {
	return e.Err
}

// LatestExecutionBlock --
func (e *EngineClient) LatestExecutionBlock(_ context.Context) (*pb.ExecutionBlock, error) {
	return e.ExecutionBlock, e.ErrLatestExecBlock
}

// ExecutionBlockByHash --
func (e *EngineClient) ExecutionBlockByHash(_ context.Context, h common.Hash, _ bool) (*pb.ExecutionBlock, error) {
	b, ok := e.BlockByHashMap[h]
	if !ok {
		return nil, errors.New("block not found")
	}
	return b, e.ErrExecBlockByHash
}

// ReconstructFullBlock --
func (e *EngineClient) ReconstructFullBlock(
	_ context.Context, blindedBlock interfaces.ReadOnlySignedBeaconBlock,
) (interfaces.SignedBeaconBlock, error) {
	if !blindedBlock.Block().IsBlinded() {
		return nil, errors.New("block must be blinded")
	}
	header, err := blindedBlock.Block().Body().Execution()
	if err != nil {
		return nil, err
	}
	payload, ok := e.ExecutionPayloadByBlockHash[bytesutil.ToBytes32(header.BlockHash())]
	if !ok {
		return nil, errors.New("block not found")
	}
	e.NumReconstructedPayloads++
	return blocks.BuildSignedBeaconBlockFromExecutionPayload(blindedBlock, payload)
}

// ReconstructFullBlockBatch --
func (e *EngineClient) ReconstructFullBlockBatch(
	ctx context.Context, blindedBlocks []interfaces.ReadOnlySignedBeaconBlock,
) ([]interfaces.SignedBeaconBlock, error) {
	fullBlocks := make([]interfaces.SignedBeaconBlock, 0, len(blindedBlocks))
	for _, b := range blindedBlocks {
		newBlock, err := e.ReconstructFullBlock(ctx, b)
		if err != nil {
			return nil, err
		}
		fullBlocks = append(fullBlocks, newBlock)
	}
	return fullBlocks, nil
}
