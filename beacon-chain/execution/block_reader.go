package execution

import (
	"context"
	"fmt"
	"math/big"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/beacon-chain/execution/types"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/monitoring/tracing"
	"go.opencensus.io/trace"
)

// searchThreshold to apply for when searching for blocks of a particular time. If the buffer
// is exceeded we recalibrate the search again.
const searchThreshold = 5

// amount of times we repeat a failed search till is satisfies the conditional.
const repeatedSearches = 2 * searchThreshold

var errBlockTimeTooLate = errors.New("provided time is later than the current execution head")

// BlockExists returns true if the block exists, its height and any possible error encountered.
func (s *Service) BlockExists(ctx context.Context, hash common.Hash) (bool, *big.Int, error) {
	ctx, span := trace.StartSpan(ctx, "execution-chain.BlockExists")
	defer span.End()

	if exists, hdrInfo, err := s.headerCache.HeaderInfoByHash(hash); exists || err != nil {
		if err != nil {
			return false, nil, err
		}
		span.AddAttributes(trace.BoolAttribute("blockCacheHit", true))
		return true, hdrInfo.Number, nil
	}
	span.AddAttributes(trace.BoolAttribute("blockCacheHit", false))
	header, err := s.HeaderByHash(ctx, hash)
	if err != nil {
		return false, big.NewInt(0), errors.Wrap(err, "could not query block with given hash")
	}

	if err := s.headerCache.AddHeader(header); err != nil {
		return false, big.NewInt(0), err
	}

	return true, new(big.Int).Set(header.Number), nil
}

// BlockHashByHeight returns the block hash of the block at the given height.
func (s *Service) BlockHashByHeight(ctx context.Context, height *big.Int) (common.Hash, error) {
	ctx, span := trace.StartSpan(ctx, "execution-chain.BlockHashByHeight")
	defer span.End()

	if exists, hInfo, err := s.headerCache.HeaderInfoByHeight(height); exists || err != nil {
		if err != nil {
			return [32]byte{}, err
		}
		span.AddAttributes(trace.BoolAttribute("headerCacheHit", true))
		return hInfo.Hash, nil
	}
	span.AddAttributes(trace.BoolAttribute("headerCacheHit", false))

	if s.rpcClient == nil {
		err := errors.New("nil rpc client")
		tracing.AnnotateError(span, err)
		return [32]byte{}, err
	}

	header, err := s.HeaderByNumber(ctx, height)
	if err != nil {
		return [32]byte{}, errors.Wrapf(err, "could not query header with height %d", height.Uint64())
	}
	if err := s.headerCache.AddHeader(header); err != nil {
		return [32]byte{}, err
	}
	return header.Hash, nil
}

// BlockTimeByHeight fetches an execution block timestamp by its height.
func (s *Service) BlockTimeByHeight(ctx context.Context, height *big.Int) (uint64, error) {
	ctx, span := trace.StartSpan(ctx, "execution-chain.BlockTimeByHeight")
	defer span.End()
	if s.rpcClient == nil {
		err := errors.New("nil rpc client")
		tracing.AnnotateError(span, err)
		return 0, err
	}

	header, err := s.HeaderByNumber(ctx, height)
	if err != nil {
		return 0, errors.Wrapf(err, "could not query block with height %d", height.Uint64())
	}
	return header.Time, nil
}

// BlockByTimestamp returns the most recent block number up to a given timestamp.
// This is an optimized version with the worst case being O(2*repeatedSearches) number of calls
// while in best case search for the block is performed in O(1).
func (s *Service) BlockByTimestamp(ctx context.Context, time uint64) (*types.HeaderInfo, error) {
	ctx, span := trace.StartSpan(ctx, "execution-chain.BlockByTimestamp")
	defer span.End()

	s.latestExecutionDataLock.RLock()
	latestBlkHeight := s.latestExecutionData.BlockHeight
	latestBlkTime := s.latestExecutionData.BlockTime
	s.latestExecutionDataLock.RUnlock()

	if time > latestBlkTime {
		return nil, errors.Wrap(errBlockTimeTooLate, fmt.Sprintf("(%d > %d)", time, latestBlkTime))
	}
	// Initialize a pointer to execution chain's history to start our search from.
	cursorNum := big.NewInt(0).SetUint64(latestBlkHeight)
	cursorTime := latestBlkTime

	numOfBlocks := uint64(0)
	estimatedBlk := cursorNum.Uint64()
	maxTimeBuffer := searchThreshold * params.BeaconConfig().SecondsPerExecutionBlock
	// Terminate if we can't find an acceptable block after
	// repeated searches.
	for range repeatedSearches {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if time > cursorTime+maxTimeBuffer {
			numOfBlocks = (time - cursorTime) / params.BeaconConfig().SecondsPerExecutionBlock
			// In the event we have an infeasible estimated block, this is a defensive
			// check to ensure it does not exceed rational bounds.
			if cursorNum.Uint64()+numOfBlocks > latestBlkHeight {
				break
			}
			estimatedBlk = cursorNum.Uint64() + numOfBlocks
		} else if time+maxTimeBuffer < cursorTime {
			numOfBlocks = (cursorTime - time) / params.BeaconConfig().SecondsPerExecutionBlock
			// In the event we have an infeasible number of blocks
			// we exit early.
			if numOfBlocks >= cursorNum.Uint64() {
				break
			}
			estimatedBlk = cursorNum.Uint64() - numOfBlocks
		} else {
			// Exit if we are in the range of
			// time - buffer <= head.time <= time + buffer
			break
		}
		hInfo, err := s.retrieveHeaderInfo(ctx, estimatedBlk)
		if err != nil {
			return nil, err
		}
		cursorNum = hInfo.Number
		cursorTime = hInfo.Time
	}

	// Exit early if we get the desired block.
	if cursorTime == time {
		return s.retrieveHeaderInfo(ctx, cursorNum.Uint64())
	}
	if cursorTime > time {
		return s.findMaxTargetExecutionBlock(ctx, big.NewInt(0).SetUint64(estimatedBlk), time)
	}
	return s.findMinTargetExecutionBlock(ctx, big.NewInt(0).SetUint64(estimatedBlk), time)
}

// Performs a search to find a target execution block which is earlier than or equal to the
// target time. This method is used when head.time > targetTime
func (s *Service) findMaxTargetExecutionBlock(ctx context.Context, upperBoundBlk *big.Int, targetTime uint64) (*types.HeaderInfo, error) {
	for bn := upperBoundBlk; ; bn = big.NewInt(0).Sub(bn, big.NewInt(1)) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		info, err := s.retrieveHeaderInfo(ctx, bn.Uint64())
		if err != nil {
			return nil, err
		}
		if info.Time <= targetTime {
			return info, nil
		}
	}
}

// Performs a search to find a target execution block which is just earlier than or equal to the
// target time. This method is used when head.time < targetTime
func (s *Service) findMinTargetExecutionBlock(ctx context.Context, lowerBoundBlk *big.Int, targetTime uint64) (*types.HeaderInfo, error) {
	for bn := lowerBoundBlk; ; bn = big.NewInt(0).Add(bn, big.NewInt(1)) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		info, err := s.retrieveHeaderInfo(ctx, bn.Uint64())
		if err != nil {
			return nil, err
		}
		// Return the last block before we hit the threshold time.
		if info.Time > targetTime {
			return s.retrieveHeaderInfo(ctx, info.Number.Uint64()-1)
		}
		// If time is equal, this is our target block.
		if info.Time == targetTime {
			return info, nil
		}
	}
}

func (s *Service) retrieveHeaderInfo(ctx context.Context, bNum uint64) (*types.HeaderInfo, error) {
	bn := big.NewInt(0).SetUint64(bNum)
	exists, info, err := s.headerCache.HeaderInfoByHeight(bn)
	if err != nil {
		return nil, err
	}
	if !exists {
		hdr, err := s.HeaderByNumber(ctx, bn)
		if err != nil {
			return nil, err
		}
		if hdr == nil {
			return nil, errors.Errorf("header with the number %d does not exist", bNum)
		}
		if err := s.headerCache.AddHeader(hdr); err != nil {
			return nil, err
		}
		info = hdr
	}
	return info, nil
}
