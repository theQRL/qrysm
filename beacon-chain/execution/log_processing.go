package execution

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	qrl "github.com/theQRL/go-qrl"
	"github.com/theQRL/go-qrl/accounts/abi/bind"
	"github.com/theQRL/go-qrl/common"
	gqrltypes "github.com/theQRL/go-qrl/core/types"
	"github.com/theQRL/qrysm/beacon-chain/cache/depositsnapshot"
	statenative "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/container/trie"
	contracts "github.com/theQRL/qrysm/contracts/deposit"
	"github.com/theQRL/qrysm/crypto/hash"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
)

var (
	depositEventSignatureHash = hash.Keccak256([]byte("DepositEvent(bytes,bytes,bytes,bytes,bytes)"))
	depositEventSignature     = common.BytesToEventSignatureLogTopic(depositEventSignatureHash[:])
)

const executionDataSavingInterval = 1000
const maxTolerableDifference = 50
const defaultExecutionHeaderReqLimit = uint64(1000)
const depositLogRequestLimit = 10000
const additiveFactorMultiplier = 0.10
const multiplicativeDecreaseDivisor = 2

var errTimedOut = errors.New("net/http: request canceled")

func tooMuchDataRequestedError(err error) bool {
	// this error is only infura specific (other providers might have different error messages)
	return err.Error() == "query returned more than 10000 results"
}

func clientTimedOutError(err error) bool {
	return strings.Contains(err.Error(), errTimedOut.Error())
}

// GenesisExecutionChainInfo retrieves the genesis time and execution block number of the beacon chain
// from the deposit contract.
func (s *Service) GenesisExecutionChainInfo() (uint64, *big.Int) {
	return s.chainStartData.GenesisTime, big.NewInt(int64(s.chainStartData.GenesisBlock))
}

// ProcessExecutionBlock processes logs from the provided execution block.
func (s *Service) ProcessExecutionBlock(ctx context.Context, blkNum *big.Int) error {
	query := qrl.FilterQuery{
		Addresses: []common.Address{
			s.cfg.depositContractAddr,
		},
		FromBlock: blkNum,
		ToBlock:   blkNum,
	}
	logs, err := s.httpLogger.FilterLogs(ctx, query)
	if err != nil {
		return err
	}
	for i, filterLog := range logs {
		// ignore logs that are not of the required block number
		if filterLog.BlockNumber != blkNum.Uint64() {
			continue
		}
		if err := s.ProcessLog(ctx, &logs[i]); err != nil {
			return errors.Wrap(err, "could not process log")
		}
	}

	return nil
}

// ProcessLog is the main method which handles the processing of all
// logs from the deposit contract on the execution chain.
func (s *Service) ProcessLog(ctx context.Context, depositLog *gqrltypes.Log) error {
	s.processingLock.RLock()
	defer s.processingLock.RUnlock()
	// Process logs according to their event signature.
	if depositLog.Topics[0] == depositEventSignature {
		if err := s.ProcessDepositLog(ctx, depositLog); err != nil {
			return errors.Wrap(err, "Could not process deposit log")
		}
		if s.lastReceivedMerkleIndex%executionDataSavingInterval == 0 {
			return s.saveExecutionChainData(ctx)
		}
		return nil
	}
	log.WithField("signature", fmt.Sprintf("%#x", depositLog.Topics[0])).Debug("Not a valid event signature")
	return nil
}

// ProcessDepositLog processes the log which had been received from
// the execution chain by trying to ascertain which participant deposited
// in the contract.
func (s *Service) ProcessDepositLog(ctx context.Context, depositLog *gqrltypes.Log) error {
	pubkey, withdrawalCredentials, amount, signature, merkleTreeIndex, err := contracts.UnpackDepositLogData(depositLog.Data)
	if err != nil {
		return errors.Wrap(err, "Could not unpack log")
	}
	// If we have already seen this Merkle index, skip processing the log.
	// This can happen sometimes when we receive the same log twice from the
	// execution network, and prevents us from updating our trie
	// with the same log twice, causing an inconsistent state root.
	index := int64(binary.LittleEndian.Uint64(merkleTreeIndex)) // lint:ignore uintcast -- MerkleTreeIndex should not exceed int64 in your lifetime.
	if index <= s.lastReceivedMerkleIndex {
		return nil
	}

	if index != s.lastReceivedMerkleIndex+1 {
		missedDepositLogsCount.Inc()
		return errors.Errorf("received incorrect merkle index: wanted %d but got %d", s.lastReceivedMerkleIndex+1, index)
	}
	s.lastReceivedMerkleIndex = index

	// We then decode the deposit input in order to create a deposit object
	// we can store in our persistent DB.
	depositData := &qrysmpb.Deposit_Data{
		Amount:                bytesutil.FromBytes8(amount),
		PublicKey:             pubkey,
		Signature:             signature,
		WithdrawalCredentials: withdrawalCredentials,
	}

	depositHash, err := depositData.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "unable to determine hashed value of deposit")
	}
	// Defensive check to validate incoming index.
	if s.depositTrie.NumOfItems() != int(index) {
		return errors.Errorf("invalid deposit index received: wanted %d but got %d", s.depositTrie.NumOfItems(), index)
	}
	if err = s.depositTrie.Insert(depositHash[:], int(index)); err != nil {
		return err
	}
	deposit := &qrysmpb.Deposit{
		Data: depositData,
	}

	// We always store all historical deposits in the DB.
	root, err := s.depositTrie.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "unable to determine root of deposit trie")
	}
	err = s.cfg.depositCache.InsertDeposit(ctx, deposit, depositLog.BlockNumber, index, root)
	if err != nil {
		return errors.Wrap(err, "unable to insert deposit into cache")
	}
	root, err = s.depositTrie.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "unable to determine root of deposit trie")
	}
	s.cfg.depositCache.InsertPendingDeposit(ctx, deposit, depositLog.BlockNumber, index, root)

	log.WithFields(logrus.Fields{
		"executionBlock":  depositLog.BlockNumber,
		"publicKey":       fmt.Sprintf("%#x", depositData.PublicKey),
		"merkleTreeIndex": index,
	}).Debug("Deposit registered from deposit contract")
	validDepositsCount.Inc()
	// Notify users what is going on, from time to time.

	if features.Get().EnableEIP4881 {
		// We finalize the trie here so that old deposits are not kept around, as they make
		// deposit tree htr computation expensive.
		dTrie, ok := s.depositTrie.(*depositsnapshot.DepositTree)
		if !ok {
			return errors.Errorf("wrong trie type initialized: %T", dTrie)
		}
		if err := dTrie.Finalize(index, depositLog.BlockHash, depositLog.BlockNumber); err != nil {
			log.WithError(err).Error("Could not finalize trie")
		}
	}

	return nil
}

// processPastLogs processes all the past logs from the deposit contract and
// updates the deposit trie with the data from each individual log.
func (s *Service) processPastLogs(ctx context.Context) error {
	s.latestExecutionDataLock.RLock()
	currentBlockNum := s.latestExecutionData.LastRequestedBlock
	s.latestExecutionDataLock.RUnlock()
	deploymentBlock := params.BeaconNetworkConfig().ContractDeploymentBlock
	// Start from the deployment block if our last requested block
	// is behind it. This is as the deposit logs can only start from the
	// block of the deployment of the deposit contract.
	if deploymentBlock > currentBlockNum {
		currentBlockNum = deploymentBlock
	}
	// To store all blocks.
	rawLogCount, err := s.depositContractCaller.GetDepositCount(&bind.CallOpts{})
	if err != nil {
		return err
	}
	logCount := binary.LittleEndian.Uint64(rawLogCount)

	latestFollowHeight, err := s.followedBlockHeight(ctx)
	if err != nil {
		return err
	}

	batchSize := s.cfg.executionHeaderReqLimit
	additiveFactor := uint64(float64(batchSize) * additiveFactorMultiplier)

	for currentBlockNum < latestFollowHeight {
		currentBlockNum, batchSize, err = s.processBlockInBatch(ctx, currentBlockNum, latestFollowHeight, batchSize, additiveFactor, logCount)
		if err != nil {
			return err
		}
	}

	s.latestExecutionDataLock.Lock()
	s.latestExecutionData.LastRequestedBlock = currentBlockNum
	s.latestExecutionDataLock.Unlock()

	c, err := s.cfg.beaconDB.FinalizedCheckpoint(ctx)
	if err != nil {
		return err
	}
	fRoot := bytesutil.ToBytes32(c.Root)
	// Return if no checkpoint exists yet.
	if fRoot == params.BeaconConfig().ZeroHash {
		return nil
	}
	fState := s.cfg.finalizedStateAtStartup
	isNil := fState == nil || fState.IsNil()

	// If processing past logs take a long time, we
	// need to check if this is the correct finalized
	// state we are referring to and whether our cached
	// finalized state is referring to our current finalized checkpoint.
	// The current code does ignore an edge case where the finalized
	// block is in a different epoch from the checkpoint's epoch.
	// This only happens in skipped slots, so pruning it is not an issue.
	if isNil || slots.ToEpoch(fState.Slot()) != c.Epoch {
		fState, err = s.cfg.stateGen.StateByRoot(ctx, fRoot)
		if err != nil {
			return err
		}
	}
	if fState != nil && !fState.IsNil() && fState.ExecutionDepositIndex() > 0 {
		s.cfg.depositCache.PrunePendingDeposits(ctx, int64(fState.ExecutionDepositIndex())) // lint:ignore uintcast -- deposit index should not exceed int64 in your lifetime.
	}
	return nil
}

func (s *Service) processBlockInBatch(ctx context.Context, currentBlockNum uint64, latestFollowHeight uint64, batchSize uint64, additiveFactor uint64, logCount uint64) (uint64, uint64, error) {
	start := currentBlockNum
	// Appropriately bound the request, as we do not
	// want request blocks beyond the current follow distance.
	end := min(currentBlockNum+batchSize, latestFollowHeight)
	query := qrl.FilterQuery{
		Addresses: []common.Address{
			s.cfg.depositContractAddr,
		},
		FromBlock: big.NewInt(0).SetUint64(start),
		ToBlock:   big.NewInt(0).SetUint64(end),
	}
	remainingLogs := logCount - uint64(s.lastReceivedMerkleIndex+1)
	// only change the end block if the remaining logs are below the required log limit.
	// reset our query and end block in this case.
	withinLimit := remainingLogs < depositLogRequestLimit
	aboveFollowHeight := end >= latestFollowHeight
	if withinLimit && aboveFollowHeight {
		query.ToBlock = big.NewInt(0).SetUint64(latestFollowHeight)
		end = latestFollowHeight
	}
	logs, err := s.httpLogger.FilterLogs(ctx, query)
	if err != nil {
		if tooMuchDataRequestedError(err) {
			if batchSize == 0 {
				return 0, 0, errors.New("batch size is zero")
			}

			// multiplicative decrease
			batchSize /= multiplicativeDecreaseDivisor
			return currentBlockNum, batchSize, nil
		}
		return 0, 0, err
	}

	s.latestExecutionDataLock.RLock()
	lastReqBlock := s.latestExecutionData.LastRequestedBlock
	s.latestExecutionDataLock.RUnlock()

	for i, filterLog := range logs {
		if filterLog.BlockNumber > currentBlockNum {
			// set new block number after checking for chainstart for previous block.
			s.latestExecutionDataLock.Lock()
			s.latestExecutionData.LastRequestedBlock = currentBlockNum
			s.latestExecutionDataLock.Unlock()
			currentBlockNum = filterLog.BlockNumber
		}
		if err := s.ProcessLog(ctx, &logs[i]); err != nil {
			// In the event the execution client gives us a garbled/bad log
			// we reset the last requested block to the previous valid block range. This
			// prevents the beacon from advancing processing of logs to another range
			// in the event of an execution client failure.
			s.latestExecutionDataLock.Lock()
			s.latestExecutionData.LastRequestedBlock = lastReqBlock
			s.latestExecutionDataLock.Unlock()
			return 0, 0, err
		}
	}

	currentBlockNum = end

	if batchSize < s.cfg.executionHeaderReqLimit {
		// update the batchSize with additive increase
		batchSize += additiveFactor
		if batchSize > s.cfg.executionHeaderReqLimit {
			batchSize = s.cfg.executionHeaderReqLimit
		}
	}
	return currentBlockNum, batchSize, nil
}

// requestBatchedHeadersAndLogs requests and processes all the headers and
// logs from the period last polled to now.
func (s *Service) requestBatchedHeadersAndLogs(ctx context.Context) error {
	// We request for the nth block behind the current head, in order to have
	// stabilized logs when we retrieve it from the execution chain.

	requestedBlock, err := s.followedBlockHeight(ctx)
	if err != nil {
		return err
	}
	s.latestExecutionDataLock.RLock()
	lastRequestedBlock := s.latestExecutionData.LastRequestedBlock
	s.latestExecutionDataLock.RUnlock()
	if requestedBlock > lastRequestedBlock &&
		requestedBlock-lastRequestedBlock > maxTolerableDifference {
		log.Infof("Falling back to historical headers and logs sync. Current difference is %d", requestedBlock-lastRequestedBlock)
		return s.processPastLogs(ctx)
	}
	for i := lastRequestedBlock + 1; i <= requestedBlock; i++ {
		// Cache execution block header here.
		_, err := s.BlockHashByHeight(ctx, big.NewInt(0).SetUint64(i))
		if err != nil {
			return err
		}
		err = s.ProcessExecutionBlock(ctx, big.NewInt(0).SetUint64(i))
		if err != nil {
			return err
		}
		s.latestExecutionDataLock.Lock()
		s.latestExecutionData.LastRequestedBlock = i
		s.latestExecutionDataLock.Unlock()
	}

	return nil
}

// saveExecutionChainData saves all execution chain related metadata to disk.
func (s *Service) saveExecutionChainData(ctx context.Context) error {
	pbState, err := statenative.ProtobufBeaconStateZond(s.preGenesisState.ToProtoUnsafe())
	if err != nil {
		return err
	}
	executionData := &qrysmpb.ExecutionChainData{
		CurrentExecutionData: s.latestExecutionData,
		ChainstartData:       s.chainStartData,
		BeaconState:          pbState, // I promise not to mutate it!
		DepositContainers:    s.cfg.depositCache.AllDepositContainers(ctx),
	}
	if features.Get().EnableEIP4881 {
		fd, err := s.cfg.depositCache.FinalizedDeposits(ctx)
		if err != nil {
			return errors.Errorf("could not get finalized deposit tree: %v", err)
		}
		tree, ok := fd.Deposits().(*depositsnapshot.DepositTree)
		if !ok {
			return errors.New("deposit tree was not EIP4881 DepositTree")
		}
		executionData.DepositSnapshot, err = tree.ToProto()
		if err != nil {
			return err
		}
	} else {
		tree, ok := s.depositTrie.(*trie.SparseMerkleTrie)
		if !ok {
			return errors.New("deposit tree was not SparseMerkleTrie")
		}
		executionData.Trie = tree.ToProto()
	}
	return s.cfg.beaconDB.SaveExecutionChainData(ctx, executionData)
}
