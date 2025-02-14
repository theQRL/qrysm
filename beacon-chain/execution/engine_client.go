package execution

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/go-zond"
	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/go-zond/common/hexutil"
	zondRPC "github.com/theQRL/go-zond/rpc"
	"github.com/theQRL/qrysm/beacon-chain/execution/types"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	payloadattribute "github.com/theQRL/qrysm/consensus-types/payload-attribute"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	pb "github.com/theQRL/qrysm/proto/engine/v1"
	"github.com/theQRL/qrysm/runtime/version"
	"go.opencensus.io/trace"
)

const (
	// NewPayloadMethodV2 v2 request string for JSON-RPC.
	NewPayloadMethodV2 = "engine_newPayloadV2"
	// ForkchoiceUpdatedMethodV2 v2 request string for JSON-RPC.
	ForkchoiceUpdatedMethodV2 = "engine_forkchoiceUpdatedV2"
	// GetPayloadMethodV2 v2 request string for JSON-RPC.
	GetPayloadMethodV2 = "engine_getPayloadV2"
	// ExecutionBlockByHashMethod request string for JSON-RPC.
	ExecutionBlockByHashMethod = "zond_getBlockByHash"
	// ExecutionBlockByNumberMethod request string for JSON-RPC.
	ExecutionBlockByNumberMethod = "zond_getBlockByNumber"
	// GetPayloadBodiesByHashV1 v1 request string for JSON-RPC.
	GetPayloadBodiesByHashV1 = "engine_getPayloadBodiesByHashV1"
	// GetPayloadBodiesByRangeV1 v1 request string for JSON-RPC.
	GetPayloadBodiesByRangeV1 = "engine_getPayloadBodiesByRangeV1"
	// Defines the seconds before timing out engine endpoints with non-block execution semantics.
	defaultEngineTimeout = 10 * time.Second
)

// ForkchoiceUpdatedResponse is the response kind received by the
// engine_forkchoiceUpdatedV1 endpoint.
type ForkchoiceUpdatedResponse struct {
	Status          *pb.PayloadStatus  `json:"payloadStatus"`
	PayloadId       *pb.PayloadIDBytes `json:"payloadId"`
	ValidationError string             `json:"validationError"`
}

// ExecutionPayloadReconstructor defines a service that can reconstruct a full beacon
// block with an execution payload from a signed beacon block and a connection
// to an execution client's engine API.
type ExecutionPayloadReconstructor interface {
	ReconstructFullBlock(
		ctx context.Context, blindedBlock interfaces.ReadOnlySignedBeaconBlock,
	) (interfaces.SignedBeaconBlock, error)
	ReconstructFullBlockBatch(
		ctx context.Context, blindedBlocks []interfaces.ReadOnlySignedBeaconBlock,
	) ([]interfaces.SignedBeaconBlock, error)
}

// EngineCaller defines a client that can interact with an Ethereum
// execution node's engine service via JSON-RPC.
type EngineCaller interface {
	NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash) ([]byte, error)
	ForkchoiceUpdated(
		ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer,
	) (*pb.PayloadIDBytes, []byte, error)
	GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (interfaces.ExecutionData, bool, error)
	ExecutionBlockByHash(ctx context.Context, hash common.Hash, withTxs bool) (*pb.ExecutionBlock, error)
}

var EmptyBlockHash = errors.New("Block hash is empty 0x0000...")

// NewPayload calls the engine_newPayloadVX method via JSON-RPC.
func (s *Service) NewPayload(ctx context.Context, payload interfaces.ExecutionData, versionedHashes []common.Hash, parentBlockRoot *common.Hash) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.NewPayload")
	defer span.End()
	start := time.Now()
	defer func() {
		newPayloadLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()

	d := time.Now().Add(time.Duration(params.BeaconConfig().ExecutionEngineTimeoutValue) * time.Second)
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()
	result := &pb.PayloadStatus{}

	switch payload.Proto().(type) {
	case *pb.ExecutionPayloadCapella:
		payloadPb, ok := payload.Proto().(*pb.ExecutionPayloadCapella)
		if !ok {
			return nil, errors.New("execution data must be a Capella execution payload")
		}
		err := s.rpcClient.CallContext(ctx, result, NewPayloadMethodV2, payloadPb)
		if err != nil {
			return nil, handleRPCError(err)
		}
	default:
		return nil, errors.New("unknown execution data type")
	}
	if result.ValidationError != "" {
		log.WithError(errors.New(result.ValidationError)).Error("Got a validation error in newPayload")
	}
	switch result.Status {
	case pb.PayloadStatus_INVALID_BLOCK_HASH:
		return nil, ErrInvalidBlockHashPayloadStatus
	case pb.PayloadStatus_ACCEPTED, pb.PayloadStatus_SYNCING:
		return nil, ErrAcceptedSyncingPayloadStatus
	case pb.PayloadStatus_INVALID:
		return result.LatestValidHash, ErrInvalidPayloadStatus
	case pb.PayloadStatus_VALID:
		return result.LatestValidHash, nil
	default:
		return nil, ErrUnknownPayloadStatus
	}
}

// ForkchoiceUpdated calls the engine_forkchoiceUpdatedV1 method via JSON-RPC.
func (s *Service) ForkchoiceUpdated(
	ctx context.Context, state *pb.ForkchoiceState, attrs payloadattribute.Attributer,
) (*pb.PayloadIDBytes, []byte, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.ForkchoiceUpdated")
	defer span.End()
	start := time.Now()
	defer func() {
		forkchoiceUpdatedLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()

	d := time.Now().Add(time.Duration(params.BeaconConfig().ExecutionEngineTimeoutValue) * time.Second)
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()
	result := &ForkchoiceUpdatedResponse{}

	if attrs == nil {
		return nil, nil, errors.New("nil payload attributer")
	}
	switch attrs.Version() {
	case version.Capella:
		a, err := attrs.PbV2()
		if err != nil {
			return nil, nil, err
		}
		err = s.rpcClient.CallContext(ctx, result, ForkchoiceUpdatedMethodV2, state, a)
		if err != nil {
			return nil, nil, handleRPCError(err)
		}
	default:
		return nil, nil, fmt.Errorf("unknown payload attribute version: %v", attrs.Version())
	}

	if result.Status == nil {
		return nil, nil, ErrNilResponse
	}
	if result.ValidationError != "" {
		log.WithError(errors.New(result.ValidationError)).Error("Got a validation error in forkChoiceUpdated")
	}
	resp := result.Status
	switch resp.Status {
	case pb.PayloadStatus_SYNCING:
		return nil, nil, ErrAcceptedSyncingPayloadStatus
	case pb.PayloadStatus_INVALID:
		return nil, resp.LatestValidHash, ErrInvalidPayloadStatus
	case pb.PayloadStatus_VALID:
		return result.PayloadId, resp.LatestValidHash, nil
	default:
		return nil, nil, ErrUnknownPayloadStatus
	}
}

// GetPayload calls the engine_getPayloadVX method via JSON-RPC.
// It returns the execution data as well as the blobs bundle.
func (s *Service) GetPayload(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (interfaces.ExecutionData, bool, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetPayload")
	defer span.End()
	start := time.Now()
	defer func() {
		getPayloadLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()

	d := time.Now().Add(defaultEngineTimeout)
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()

	result := &pb.ExecutionPayloadCapellaWithValue{}
	err := s.rpcClient.CallContext(ctx, result, GetPayloadMethodV2, pb.PayloadIDBytes(payloadId))
	if err != nil {
		return nil, false, handleRPCError(err)
	}
	ed, err := blocks.WrappedExecutionPayloadCapella(result.Payload, blocks.PayloadValueToGwei(result.Value))
	if err != nil {
		return nil, false, err
	}
	return ed, false, nil
}

// LatestExecutionBlock fetches the latest execution engine block by calling
// zond_blockByNumber via JSON-RPC.
func (s *Service) LatestExecutionBlock(ctx context.Context) (*pb.ExecutionBlock, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.LatestExecutionBlock")
	defer span.End()

	result := &pb.ExecutionBlock{}
	err := s.rpcClient.CallContext(
		ctx,
		result,
		ExecutionBlockByNumberMethod,
		"latest",
		false, /* no full transaction objects */
	)
	return result, handleRPCError(err)
}

// ExecutionBlockByHash fetches an execution engine block by hash by calling
// zond_blockByHash via JSON-RPC.
func (s *Service) ExecutionBlockByHash(ctx context.Context, hash common.Hash, withTxs bool) (*pb.ExecutionBlock, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.ExecutionBlockByHash")
	defer span.End()
	result := &pb.ExecutionBlock{}
	err := s.rpcClient.CallContext(ctx, result, ExecutionBlockByHashMethod, hash, withTxs)
	return result, handleRPCError(err)
}

// ExecutionBlocksByHashes fetches a batch of execution engine blocks by hash by calling
// zond_blockByHash via JSON-RPC.
func (s *Service) ExecutionBlocksByHashes(ctx context.Context, hashes []common.Hash, withTxs bool) ([]*pb.ExecutionBlock, error) {
	_, span := trace.StartSpan(ctx, "powchain.engine-api-client.ExecutionBlocksByHashes")
	defer span.End()
	numOfHashes := len(hashes)
	elems := make([]zondRPC.BatchElem, 0, numOfHashes)
	execBlks := make([]*pb.ExecutionBlock, 0, numOfHashes)
	if numOfHashes == 0 {
		return execBlks, nil
	}
	for _, h := range hashes {
		blk := &pb.ExecutionBlock{}
		newH := h
		elems = append(elems, zondRPC.BatchElem{
			Method: ExecutionBlockByHashMethod,
			Args:   []interface{}{newH, withTxs},
			Result: blk,
			Error:  error(nil),
		})
		execBlks = append(execBlks, blk)
	}
	ioErr := s.rpcClient.BatchCall(elems)
	if ioErr != nil {
		return nil, ioErr
	}
	for _, e := range elems {
		if e.Error != nil {
			return nil, handleRPCError(e.Error)
		}
	}
	return execBlks, nil
}

// HeaderByHash returns the relevant header details for the provided block hash.
func (s *Service) HeaderByHash(ctx context.Context, hash common.Hash) (*types.HeaderInfo, error) {
	var hdr *types.HeaderInfo
	err := s.rpcClient.CallContext(ctx, &hdr, ExecutionBlockByHashMethod, hash, false /* no transactions */)
	if err == nil && hdr == nil {
		err = zond.NotFound
	}
	return hdr, err
}

// HeaderByNumber returns the relevant header details for the provided block number.
func (s *Service) HeaderByNumber(ctx context.Context, number *big.Int) (*types.HeaderInfo, error) {
	var hdr *types.HeaderInfo
	err := s.rpcClient.CallContext(ctx, &hdr, ExecutionBlockByNumberMethod, toBlockNumArg(number), false /* no transactions */)
	if err == nil && hdr == nil {
		err = zond.NotFound
	}
	return hdr, err
}

// GetPayloadBodiesByHash returns the relevant payload bodies for the provided block hash.
func (s *Service) GetPayloadBodiesByHash(ctx context.Context, executionBlockHashes []common.Hash) ([]*pb.ExecutionPayloadBodyV1, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetPayloadBodiesByHashV1")
	defer span.End()

	result := make([]*pb.ExecutionPayloadBodyV1, 0)
	err := s.rpcClient.CallContext(ctx, &result, GetPayloadBodiesByHashV1, executionBlockHashes)

	for i, item := range result {
		if item == nil {
			result[i] = &pb.ExecutionPayloadBodyV1{
				Transactions: make([][]byte, 0),
				Withdrawals:  make([]*pb.Withdrawal, 0),
			}
		}
	}
	return result, handleRPCError(err)
}

// GetPayloadBodiesByRange returns the relevant payload bodies for the provided range.
func (s *Service) GetPayloadBodiesByRange(ctx context.Context, start, count uint64) ([]*pb.ExecutionPayloadBodyV1, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetPayloadBodiesByRangeV1")
	defer span.End()

	result := make([]*pb.ExecutionPayloadBodyV1, 0)
	err := s.rpcClient.CallContext(ctx, &result, GetPayloadBodiesByRangeV1, start, count)

	for i, item := range result {
		if item == nil {
			result[i] = &pb.ExecutionPayloadBodyV1{
				Transactions: make([][]byte, 0),
				Withdrawals:  make([]*pb.Withdrawal, 0),
			}
		}
	}
	return result, handleRPCError(err)
}

// ReconstructFullBlock takes in a blinded beacon block and reconstructs
// a beacon block with a full execution payload via the engine API.
func (s *Service) ReconstructFullBlock(
	ctx context.Context, blindedBlock interfaces.ReadOnlySignedBeaconBlock,
) (interfaces.SignedBeaconBlock, error) {
	if err := blocks.BeaconBlockIsNil(blindedBlock); err != nil {
		return nil, errors.Wrap(err, "cannot reconstruct bellatrix block from nil data")
	}
	if !blindedBlock.Block().IsBlinded() {
		return nil, errors.New("can only reconstruct block from blinded block format")
	}
	header, err := blindedBlock.Block().Body().Execution()
	if err != nil {
		return nil, err
	}
	if header.IsNil() {
		return nil, errors.New("execution payload header in blinded block was nil")
	}

	executionBlockHash := common.BytesToHash(header.BlockHash())
	payload, err := s.retrievePayloadFromExecutionHash(ctx, executionBlockHash, header, blindedBlock.Version())
	if err != nil {
		return nil, err
	}
	fullBlock, err := blocks.BuildSignedBeaconBlockFromExecutionPayload(blindedBlock, payload.Proto())
	if err != nil {
		return nil, err
	}
	reconstructedExecutionPayloadCount.Add(1)
	return fullBlock, nil
}

// ReconstructFullBlockBatch takes in a batch of blinded beacon blocks and reconstructs
// them with a full execution payload for each block via the engine API.
func (s *Service) ReconstructFullBlockBatch(
	ctx context.Context, blindedBlocks []interfaces.ReadOnlySignedBeaconBlock,
) ([]interfaces.SignedBeaconBlock, error) {
	if len(blindedBlocks) == 0 {
		return []interfaces.SignedBeaconBlock{}, nil
	}
	executionHashes := []common.Hash{}
	validExecPayloads := []int{}
	for i, b := range blindedBlocks {
		if err := blocks.BeaconBlockIsNil(b); err != nil {
			return nil, errors.Wrap(err, "cannot reconstruct bellatrix block from nil data")
		}
		if !b.Block().IsBlinded() {
			return nil, errors.New("can only reconstruct block from blinded block format")
		}
		header, err := b.Block().Body().Execution()
		if err != nil {
			return nil, err
		}
		if header.IsNil() {
			return nil, errors.New("execution payload header in blinded block was nil")
		}

		executionBlockHash := common.BytesToHash(header.BlockHash())
		validExecPayloads = append(validExecPayloads, i)
		executionHashes = append(executionHashes, executionBlockHash)
	}
	fullBlocks, err := s.retrievePayloadsFromExecutionHashes(ctx, executionHashes, validExecPayloads, blindedBlocks)
	if err != nil {
		return nil, err
	}

	reconstructedExecutionPayloadCount.Add(float64(len(blindedBlocks)))
	return fullBlocks, nil
}

func (s *Service) retrievePayloadFromExecutionHash(ctx context.Context, executionBlockHash common.Hash, header interfaces.ExecutionData, version int) (interfaces.ExecutionData, error) {
	if features.Get().EnableOptionalEngineMethods {
		pBodies, err := s.GetPayloadBodiesByHash(ctx, []common.Hash{executionBlockHash})
		if err != nil {
			return nil, fmt.Errorf("could not get payload body by hash %#x: %v", executionBlockHash, err)
		}
		if len(pBodies) != 1 {
			return nil, errors.Errorf("could not retrieve the correct number of payload bodies: wanted 1 but got %d", len(pBodies))
		}
		bdy := pBodies[0]
		return fullPayloadFromPayloadBody(header, bdy, version)
	}

	executionBlock, err := s.ExecutionBlockByHash(ctx, executionBlockHash, true /* with txs */)
	if err != nil {
		return nil, fmt.Errorf("could not fetch execution block with txs by hash %#x: %v", executionBlockHash, err)
	}
	if executionBlock == nil {
		return nil, fmt.Errorf("received nil execution block for request by hash %#x", executionBlockHash)
	}
	if bytes.Equal(executionBlock.Hash.Bytes(), []byte{}) {
		return nil, EmptyBlockHash
	}

	executionBlock.Version = version
	return fullPayloadFromExecutionBlock(version, header, executionBlock)
}

func (s *Service) retrievePayloadsFromExecutionHashes(
	ctx context.Context,
	executionHashes []common.Hash,
	validExecPayloads []int,
	blindedBlocks []interfaces.ReadOnlySignedBeaconBlock) ([]interfaces.SignedBeaconBlock, error) {
	fullBlocks := make([]interfaces.SignedBeaconBlock, len(blindedBlocks))
	var execBlocks []*pb.ExecutionBlock
	var payloadBodies []*pb.ExecutionPayloadBodyV1
	var err error
	if features.Get().EnableOptionalEngineMethods {
		payloadBodies, err = s.GetPayloadBodiesByHash(ctx, executionHashes)
		if err != nil {
			return nil, fmt.Errorf("could not fetch payload bodies by hash %#x: %v", executionHashes, err)
		}
	} else {
		execBlocks, err = s.ExecutionBlocksByHashes(ctx, executionHashes, true /* with txs*/)
		if err != nil {
			return nil, fmt.Errorf("could not fetch execution blocks with txs by hash %#x: %v", executionHashes, err)
		}
	}

	// For each valid payload, we reconstruct the full block from it with the
	// blinded block.
	for sliceIdx, realIdx := range validExecPayloads {
		var payload interfaces.ExecutionData
		bblock := blindedBlocks[realIdx]
		if features.Get().EnableOptionalEngineMethods {
			b := payloadBodies[sliceIdx]
			if b == nil {
				return nil, fmt.Errorf("received nil payload body for request by hash %#x", executionHashes[sliceIdx])
			}
			header, err := bblock.Block().Body().Execution()
			if err != nil {
				return nil, err
			}
			payload, err = fullPayloadFromPayloadBody(header, b, bblock.Version())
			if err != nil {
				return nil, err
			}
		} else {
			b := execBlocks[sliceIdx]
			if b == nil {
				return nil, fmt.Errorf("received nil execution block for request by hash %#x", executionHashes[sliceIdx])
			}
			header, err := bblock.Block().Body().Execution()
			if err != nil {
				return nil, err
			}
			payload, err = fullPayloadFromExecutionBlock(bblock.Version(), header, b)
			if err != nil {
				return nil, err
			}
		}
		fullBlock, err := blocks.BuildSignedBeaconBlockFromExecutionPayload(bblock, payload.Proto())
		if err != nil {
			return nil, err
		}
		fullBlocks[realIdx] = fullBlock
	}
	return fullBlocks, nil
}

func fullPayloadFromExecutionBlock(
	blockVersion int, header interfaces.ExecutionData, block *pb.ExecutionBlock,
) (interfaces.ExecutionData, error) {
	if header.IsNil() || block == nil {
		return nil, errors.New("execution block and header cannot be nil")
	}
	blockHash := block.Hash
	if !bytes.Equal(header.BlockHash(), blockHash[:]) {
		return nil, fmt.Errorf(
			"block hash field in execution header %#x does not match execution block hash %#x",
			header.BlockHash(),
			blockHash,
		)
	}
	blockTransactions := block.Transactions
	txs := make([][]byte, len(blockTransactions))
	for i, tx := range blockTransactions {
		txBin, err := tx.MarshalBinary()
		if err != nil {
			return nil, err
		}
		txs[i] = txBin
	}

	switch blockVersion {
	case version.Capella:
		return blocks.WrappedExecutionPayloadCapella(&pb.ExecutionPayloadCapella{
			ParentHash:    header.ParentHash(),
			FeeRecipient:  header.FeeRecipient(),
			StateRoot:     header.StateRoot(),
			ReceiptsRoot:  header.ReceiptsRoot(),
			LogsBloom:     header.LogsBloom(),
			PrevRandao:    header.PrevRandao(),
			BlockNumber:   header.BlockNumber(),
			GasLimit:      header.GasLimit(),
			GasUsed:       header.GasUsed(),
			Timestamp:     header.Timestamp(),
			ExtraData:     header.ExtraData(),
			BaseFeePerGas: header.BaseFeePerGas(),
			BlockHash:     blockHash[:],
			Transactions:  txs,
			Withdrawals:   block.Withdrawals,
		}, 0) // We can't get the block value and don't care about the block value for this instance
	default:
		return nil, fmt.Errorf("unknown execution block version %d", block.Version)
	}
}

func fullPayloadFromPayloadBody(
	header interfaces.ExecutionData, body *pb.ExecutionPayloadBodyV1, bVersion int,
) (interfaces.ExecutionData, error) {
	if header.IsNil() || body == nil {
		return nil, errors.New("execution block and header cannot be nil")
	}

	switch bVersion {
	case version.Capella:
		return blocks.WrappedExecutionPayloadCapella(&pb.ExecutionPayloadCapella{
			ParentHash:    header.ParentHash(),
			FeeRecipient:  header.FeeRecipient(),
			StateRoot:     header.StateRoot(),
			ReceiptsRoot:  header.ReceiptsRoot(),
			LogsBloom:     header.LogsBloom(),
			PrevRandao:    header.PrevRandao(),
			BlockNumber:   header.BlockNumber(),
			GasLimit:      header.GasLimit(),
			GasUsed:       header.GasUsed(),
			Timestamp:     header.Timestamp(),
			ExtraData:     header.ExtraData(),
			BaseFeePerGas: header.BaseFeePerGas(),
			BlockHash:     header.BlockHash(),
			Transactions:  body.Transactions,
			Withdrawals:   body.Withdrawals,
		}, 0) // We can't get the block value and don't care about the block value for this instance
	default:
		return nil, fmt.Errorf("unknown execution block version for payload %d", bVersion)
	}
}

// Handles errors received from the RPC server according to the specification.
func handleRPCError(err error) error {
	if err == nil {
		return nil
	}
	if isTimeout(err) {
		return ErrHTTPTimeout
	}
	e, ok := err.(zondRPC.Error)
	if !ok {
		// TODO(now.youtrack.cloud/issue/TQ-1)
		if strings.Contains(err.Error(), "401 Unauthorized") {
			log.Error("HTTP authentication to your execution client is not working. Please ensure " +
				"you are setting a correct value for the --jwt-secret flag in Qrysm, or use an IPC connection if on " +
				"the same machine. Please see our documentation for more information on authenticating connections " +
				"here https://docs.prylabs.network/docs/execution-node/authentication")
			return fmt.Errorf("could not authenticate connection to execution client: %v", err)
		}
		return errors.Wrapf(err, "got an unexpected error in JSON-RPC response")
	}
	switch e.ErrorCode() {
	case -32700:
		errParseCount.Inc()
		return ErrParse
	case -32600:
		errInvalidRequestCount.Inc()
		return ErrInvalidRequest
	case -32601:
		errMethodNotFoundCount.Inc()
		return ErrMethodNotFound
	case -32602:
		errInvalidParamsCount.Inc()
		return ErrInvalidParams
	case -32603:
		errInternalCount.Inc()
		return ErrInternal
	case -38001:
		errUnknownPayloadCount.Inc()
		return ErrUnknownPayload
	case -38002:
		errInvalidForkchoiceStateCount.Inc()
		return ErrInvalidForkchoiceState
	case -38003:
		errInvalidPayloadAttributesCount.Inc()
		return ErrInvalidPayloadAttributes
	case -38004:
		errRequestTooLargeCount.Inc()
		return ErrRequestTooLarge
	case -32000:
		errServerErrorCount.Inc()
		// Only -32000 status codes are data errors in the RPC specification.
		errWithData, ok := err.(zondRPC.DataError)
		if !ok {
			return errors.Wrapf(err, "got an unexpected error in JSON-RPC response")
		}
		return errors.Wrapf(ErrServer, "%v", errWithData.Error())
	default:
		return err
	}
}

// ErrHTTPTimeout returns true if the error is a http.Client timeout error.
var ErrHTTPTimeout = errors.New("timeout from http.Client")

type httpTimeoutError interface {
	Error() string
	Timeout() bool
}

func isTimeout(e error) bool {
	t, ok := e.(httpTimeoutError)
	return ok && t.Timeout()
}

func toBlockNumArg(number *big.Int) string {
	if number == nil {
		return "latest"
	}
	pending := big.NewInt(-1)
	if number.Cmp(pending) == 0 {
		return "pending"
	}
	finalized := big.NewInt(int64(zondRPC.FinalizedBlockNumber))
	if number.Cmp(finalized) == 0 {
		return "finalized"
	}
	safe := big.NewInt(int64(zondRPC.SafeBlockNumber))
	if number.Cmp(safe) == 0 {
		return "safe"
	}
	return hexutil.EncodeBig(number)
}
