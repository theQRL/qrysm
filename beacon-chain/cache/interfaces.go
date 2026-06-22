package cache

import (
	"context"
	"math/big"

	"github.com/theQRL/go-qrl/common"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// DepositCache combines the interfaces for retrieving and inserting deposit information.
type DepositCache interface {
	DepositFetcher
	DepositInserter
}

// DepositFetcher defines a struct which can retrieve deposit information from a store.
type DepositFetcher interface {
	AllDeposits(ctx context.Context, untilBlk *big.Int) []*qrysmpb.Deposit
	AllDepositContainers(ctx context.Context) []*qrysmpb.DepositContainer
	DepositByPubkey(ctx context.Context, pubKey []byte) (*qrysmpb.Deposit, *big.Int)
	DepositsNumberAndRootAtHeight(ctx context.Context, blockHeight *big.Int) (uint64, [32]byte)
	InsertPendingDeposit(ctx context.Context, d *qrysmpb.Deposit, blockNum uint64, index int64, depositRoot [32]byte)
	PendingDeposits(ctx context.Context, untilBlk *big.Int) []*qrysmpb.Deposit
	PendingContainers(ctx context.Context, untilBlk *big.Int) []*qrysmpb.DepositContainer
	PrunePendingDeposits(ctx context.Context, merkleTreeIndex int64)
	PruneProofs(ctx context.Context, untilDepositIndex int64) error
	FinalizedFetcher
}

// DepositInserter defines a struct which can insert deposit information from a store.
type DepositInserter interface {
	InsertDeposit(ctx context.Context, d *qrysmpb.Deposit, blockNum uint64, index int64, depositRoot [32]byte) error
	InsertDepositContainers(ctx context.Context, ctrs []*qrysmpb.DepositContainer)
	InsertFinalizedDeposits(ctx context.Context, executionDepositIndex int64, executionHash common.Hash, executionNumber uint64) error
}

// FinalizedFetcher is a smaller interface defined to be the bare minimum to satisfy “Service”.
// It extends the "DepositFetcher" interface with additional methods for fetching finalized deposits.
type FinalizedFetcher interface {
	FinalizedDeposits(ctx context.Context) (FinalizedDeposits, error)
	NonFinalizedDeposits(ctx context.Context, lastFinalizedIndex int64, untilBlk *big.Int) []*qrysmpb.Deposit
}

// FinalizedDeposits defines a method to access a merkle tree containing deposits and their indexes.
type FinalizedDeposits interface {
	Deposits() MerkleTree
	MerkleTrieIndex() int64
}

// MerkleTree defines methods for constructing and manipulating a merkle tree.
type MerkleTree interface {
	HashTreeRoot() ([32]byte, error)
	NumOfItems() int
	Insert(item []byte, index int) error
	MerkleProof(index int) ([][]byte, error)
}
