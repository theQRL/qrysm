package util

import (
	"math/big"
	"testing"

	"github.com/golang/snappy"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/core/types"
	"github.com/theQRL/go-qrl/crypto/pqcrypto/wallet"
	qrlparams "github.com/theQRL/go-qrl/params"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	qrysmTime "github.com/theQRL/qrysm/time"
)

// TestMaxBlockSize_FullGasLimit builds a beacon block whose execution payload
// contains signed go-qrl transactions that together consume exactly the 20M
// gas limit (params.MaxGasLimit). It then SSZ-encodes the block and reports
// the total wire size.
func TestMaxBlockSize_FullGasLimit(t *testing.T) {
	const (
		maxGas      = qrlparams.MaxGasLimit   // 20_000_000
		txGas       = qrlparams.TxGas         // 21_000 per simple transfer
		zeroGasCost = qrlparams.TxDataZeroGas // 4 per zero byte of tx data
	)

	// We want to exactly hit maxGas.
	// 951 simple transfers = 951 × 21_000 = 19_971_000
	// Remaining gas = 20_000_000 − 19_971_000 = 29_000
	// Last tx: 21_000 base + dataGas = 29_000 → dataGas = 8_000
	// Using zero bytes at 4 gas each: 8_000 / 4 = 2_000 zero bytes
	const (
		numSimpleTxs = 951
		lastTxGas    = maxGas - (numSimpleTxs * txGas)   // 29_000
		lastTxData   = (lastTxGas - txGas) / zeroGasCost // 2_000 zero bytes
	)

	totalGas := numSimpleTxs*txGas + lastTxGas
	require.Equal(t, maxGas, totalGas, "total gas must equal MaxGasLimit")

	// Generate a wallet for signing transactions.
	w, err := wallet.Generate(wallet.ML_DSA_87)
	require.NoError(t, err)

	chainID := big.NewInt(1)
	signer := types.NewZondSigner(chainID)
	recipient := common.Address{0x01}

	var encodedTxs [][]byte
	var gasUsed uint64

	// Create 951 simple transfer transactions.
	for i := uint64(0); i < numSimpleTxs; i++ {
		tx, err := types.SignNewTx(w, signer, &types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     i,
			GasTipCap: big.NewInt(1),
			GasFeeCap: big.NewInt(1000000000), // 1 Gwei
			Gas:       txGas,
			To:        &recipient,
			Value:     big.NewInt(0),
		})
		require.NoError(t, err, "failed to sign simple tx %d", i)

		raw, err := tx.MarshalBinary()
		require.NoError(t, err)
		encodedTxs = append(encodedTxs, raw)
		gasUsed += txGas
	}

	// Create the last transaction with zero-byte data to consume remaining gas.
	{
		data := make([]byte, lastTxData) // all zeros → 4 gas/byte
		tx, err := types.SignNewTx(w, signer, &types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     numSimpleTxs,
			GasTipCap: big.NewInt(1),
			GasFeeCap: big.NewInt(1000000000),
			Gas:       lastTxGas,
			To:        &recipient,
			Value:     big.NewInt(0),
			Data:      data,
		})
		require.NoError(t, err, "failed to sign padded tx")

		raw, err := tx.MarshalBinary()
		require.NoError(t, err)
		encodedTxs = append(encodedTxs, raw)
		gasUsed += lastTxGas
	}

	require.Equal(t, uint64(maxGas), gasUsed, "gasUsed must equal MaxGasLimit")
	t.Logf("Total transactions: %d", len(encodedTxs))
	t.Logf("Gas consumed: %d / %d", gasUsed, maxGas)

	// Build a hydrated signed beacon block with the transactions in the execution payload.
	block := HydrateSignedBeaconBlockZond(&qrysmpb.SignedBeaconBlockZond{
		Block: &qrysmpb.BeaconBlockZond{
			Body: &qrysmpb.BeaconBlockBodyZond{
				ExecutionPayload: &enginev1.ExecutionPayloadZond{
					ParentHash:    make([]byte, fieldparams.RootLength),
					FeeRecipient:  make([]byte, 64),
					StateRoot:     make([]byte, fieldparams.RootLength),
					ReceiptsRoot:  make([]byte, fieldparams.RootLength),
					LogsBloom:     make([]byte, 256),
					PrevRandao:    make([]byte, fieldparams.RootLength),
					BaseFeePerGas: make([]byte, fieldparams.RootLength),
					BlockHash:     make([]byte, fieldparams.RootLength),
					ExtraData:     make([]byte, 0),
					GasLimit:      maxGas,
					GasUsed:       gasUsed,
					Transactions:  encodedTxs,
					Withdrawals:   make([]*enginev1.Withdrawal, 0),
				},
			},
		},
	})

	// SSZ-encode the full signed beacon block and report size.
	sszBytes, err := block.MarshalSSZ()
	require.NoError(t, err)

	t.Logf("Signed beacon block SSZ size: %d bytes (%.2f MB)", len(sszBytes), float64(len(sszBytes))/(1024*1024))

	compressed := snappy.Encode(nil, sszBytes)
	t.Logf("Snappy compressed (p2p wire size): %d bytes (%.2f MB), ratio: %.1f%%",
		len(compressed), float64(len(compressed))/(1024*1024), float64(len(compressed))/float64(len(sszBytes))*100)

	// Verify round-trip: unmarshal and check gas fields.
	decoded := &qrysmpb.SignedBeaconBlockZond{}
	require.NoError(t, decoded.UnmarshalSSZ(sszBytes))
	require.Equal(t, maxGas, decoded.Block.Body.ExecutionPayload.GasLimit)
	require.Equal(t, maxGas, decoded.Block.Body.ExecutionPayload.GasUsed)
	require.Equal(t, len(encodedTxs), len(decoded.Block.Body.ExecutionPayload.Transactions))
}

// TestMaxBlockSize_FullBlock builds a worst-case beacon block with:
//   - 128 sync committee signatures (full SyncAggregate)
//   - 128 attestations (MaxAttestations) with all validators attesting
//   - Block proposer signature
//   - Execution payload filled with signed go-qrl transactions consuming 20M gas
//
// It uses GenerateFullBlockZond for valid sync committee, attestations, and proposer,
// then replaces execution payload transactions with real signed go-qrl transactions.
func TestMaxBlockSize_FullBlock(t *testing.T) {
	const (
		maxGas      = qrlparams.MaxGasLimit   // 20_000_000
		txGas       = qrlparams.TxGas         // 21_000 per simple transfer
		zeroGasCost = qrlparams.TxDataZeroGas // 4 per zero byte of tx data
	)
	const (
		numSimpleTxs = 951
		lastTxGas    = maxGas - (numSimpleTxs * txGas)   // 29_000
		lastTxData   = (lastTxGas - txGas) / zeroGasCost // 2_000 zero bytes
	)

	// 16384 validators → 1 committee per slot with 128 validators → 128 attestations.
	// committeesPerSlot = 16384 / SlotsPerEpoch(128) / TargetCommitteeSize(128) = 1
	// committeeSize     = 16384 / SlotsPerEpoch(128) = 128
	numValidators := uint64(16384)
	t.Logf("Generating genesis state with %d validators...", numValidators)
	genesis, keys := DeterministicGenesisStateZond(t, numValidators)

	// Set genesis time far in the past so slot 1 is valid.
	genesisTime := uint64(qrysmTime.Now().Unix()) - 90000000
	require.NoError(t, genesis.SetGenesisTime(genesisTime))

	// Generate a full block with max attestations and full sync aggregate.
	conf := &BlockGenConfig{
		NumAttestations:   params.BeaconConfig().MaxAttestations, // 128
		FullSyncAggregate: true,
		NumTransactions:   0,
	}
	t.Logf("Generating full block (attestations=%d, fullSyncAggregate=true)...", conf.NumAttestations)
	block, err := GenerateFullBlockZond(genesis, keys, conf, 1)
	require.NoError(t, err)

	// Report attestation and sync committee details.
	numAtts := len(block.Block.Body.Attestations)
	numSyncSigs := len(block.Block.Body.SyncAggregate.SyncCommitteeSignatures)
	t.Logf("Attestations: %d, Sync committee signatures: %d, Proposer index: %d",
		numAtts, numSyncSigs, block.Block.ProposerIndex)

	// Build signed go-qrl transactions that consume exactly 20M gas.
	w, err := wallet.Generate(wallet.ML_DSA_87)
	require.NoError(t, err)

	chainID := big.NewInt(1)
	signer := types.NewZondSigner(chainID)
	recipient := common.Address{0x01}

	var encodedTxs [][]byte
	var gasUsed uint64

	for i := uint64(0); i < numSimpleTxs; i++ {
		tx, err := types.SignNewTx(w, signer, &types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     i,
			GasTipCap: big.NewInt(1),
			GasFeeCap: big.NewInt(1000000000),
			Gas:       txGas,
			To:        &recipient,
			Value:     big.NewInt(0),
		})
		require.NoError(t, err)
		raw, err := tx.MarshalBinary()
		require.NoError(t, err)
		encodedTxs = append(encodedTxs, raw)
		gasUsed += txGas
	}
	{
		data := make([]byte, lastTxData)
		tx, err := types.SignNewTx(w, signer, &types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     numSimpleTxs,
			GasTipCap: big.NewInt(1),
			GasFeeCap: big.NewInt(1000000000),
			Gas:       lastTxGas,
			To:        &recipient,
			Value:     big.NewInt(0),
			Data:      data,
		})
		require.NoError(t, err)
		raw, err := tx.MarshalBinary()
		require.NoError(t, err)
		encodedTxs = append(encodedTxs, raw)
		gasUsed += lastTxGas
	}
	require.Equal(t, uint64(maxGas), gasUsed)
	t.Logf("Transactions: %d, Gas: %d / %d", len(encodedTxs), gasUsed, maxGas)

	// Inject transactions into the execution payload.
	block.Block.Body.ExecutionPayload.Transactions = encodedTxs
	block.Block.Body.ExecutionPayload.GasLimit = maxGas
	block.Block.Body.ExecutionPayload.GasUsed = gasUsed

	// SSZ-encode and measure.
	sszBytes, err := block.MarshalSSZ()
	require.NoError(t, err)

	t.Logf("Signed beacon block SSZ size: %d bytes (%.2f MB)", len(sszBytes), float64(len(sszBytes))/(1024*1024))

	compressed := snappy.Encode(nil, sszBytes)
	t.Logf("Snappy compressed (p2p wire size): %d bytes (%.2f MB), ratio: %.1f%%",
		len(compressed), float64(len(compressed))/(1024*1024), float64(len(compressed))/float64(len(sszBytes))*100)

	// Verify round-trip.
	decoded := &qrysmpb.SignedBeaconBlockZond{}
	require.NoError(t, decoded.UnmarshalSSZ(sszBytes))
	require.Equal(t, maxGas, decoded.Block.Body.ExecutionPayload.GasLimit)
	require.Equal(t, maxGas, decoded.Block.Body.ExecutionPayload.GasUsed)
	require.Equal(t, len(encodedTxs), len(decoded.Block.Body.ExecutionPayload.Transactions))
	require.Equal(t, numAtts, len(decoded.Block.Body.Attestations))
	require.Equal(t, numSyncSigs, len(decoded.Block.Body.SyncAggregate.SyncCommitteeSignatures))
}
