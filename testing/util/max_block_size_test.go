package util

import (
	"math/big"
	"testing"

	"github.com/golang/snappy"
	"github.com/theQRL/go-bitfield"
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

// fullGasLimitTransactions builds signed go-qrl transactions that together
// consume exactly qrlparams.MaxGasLimit. Simple ML-DSA-signed transfers are the
// densest way to spend gas in bytes (roughly 0.35 B/gas), so this is the
// largest execution payload a block can legally carry.
func fullGasLimitTransactions(t *testing.T) (encodedTxs [][]byte, gasUsed uint64) {
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
	return encodedTxs, gasUsed
}

// TestMaxBlockSize_FullGasLimit builds a beacon block whose execution payload
// contains signed go-qrl transactions that together consume exactly the 20M
// gas limit (params.MaxGasLimit). It then SSZ-encodes the block and reports
// the total wire size.
func TestMaxBlockSize_FullGasLimit(t *testing.T) {
	const maxGas = qrlparams.MaxGasLimit
	encodedTxs, gasUsed := fullGasLimitTransactions(t)

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

// TestMaxBlockSize_FullBlock builds a large beacon block with:
//   - 128 sync committee signatures (full SyncAggregate)
//   - MaxAttestations attestations that together cover the slot's whole
//     committee, sized at MaxValidatorsPerCommittee (the widest the SSZ types allow)
//   - Block proposer signature
//   - Execution payload filled with signed go-qrl transactions consuming 20M gas
//
// It uses GenerateFullBlockZond for valid sync committee, attestations, and proposer,
// then replaces execution payload transactions with real signed go-qrl transactions.
func TestMaxBlockSize_FullBlock(t *testing.T) {
	const maxGas = qrlparams.MaxGasLimit

	// Size the validator set so each slot's single committee is exactly
	// MaxValidatorsPerCommittee wide, the widest committee the SSZ types can encode:
	// committeeSize = numValidators / SlotsPerEpoch = MaxValidatorsPerCommittee.
	cfg := params.BeaconConfig()
	numValidators := cfg.MaxValidatorsPerCommittee * uint64(cfg.SlotsPerEpoch)
	t.Logf("Generating genesis state with %d validators...", numValidators)
	genesis, keys := DeterministicGenesisStateZond(t, numValidators)

	// Set genesis time far in the past so slot 1 is valid.
	genesisTime := uint64(qrysmTime.Now().Unix()) - 90000000
	require.NoError(t, genesis.SetGenesisTime(genesisTime))

	// Generate a full block with max attestations and full sync aggregate.
	conf := &BlockGenConfig{
		NumAttestations:   params.BeaconConfig().MaxAttestations,
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

	// Build signed go-qrl transactions that consume exactly 20M gas and
	// inject them into the execution payload.
	encodedTxs, gasUsed := fullGasLimitTransactions(t)
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

// worstCaseBlockLimits are the per-list sizes used by worstCaseSignedBlock. They
// default to the beacon config so the block tracks future parameter changes;
// tests nudge a single field past its bound to prove the SSZ types enforce it.
type worstCaseBlockLimits struct {
	attestations           uint64
	validatorsPerCommittee uint64
}

func configWorstCaseLimits() worstCaseBlockLimits {
	cfg := params.BeaconConfig()
	return worstCaseBlockLimits{
		attestations:           cfg.MaxAttestations,
		validatorsPerCommittee: cfg.MaxValidatorsPerCommittee,
	}
}

// worstCaseSignedBlock builds the largest signed block the beacon config and
// the SSZ types admit: every body list at its maximum, every attestation and
// indexed attestation carrying a signature for every committee seat, a full
// sync aggregate, and the supplied execution payload transactions. Signatures
// are zero-filled; only the encoded size matters here.
func worstCaseSignedBlock(t *testing.T, lim worstCaseBlockLimits, txs [][]byte, gasUsed uint64) *qrysmpb.SignedBeaconBlockZond {
	cfg := params.BeaconConfig()
	sig := func() []byte { return make([]byte, fieldparams.MLDSA87SignatureLength) }
	sigs := func(n uint64) [][]byte {
		out := make([][]byte, n)
		for i := range out {
			out[i] = sig()
		}
		return out
	}

	mkAtt := func() *qrysmpb.Attestation {
		bits := bitfield.NewBitlist(lim.validatorsPerCommittee)
		for i := uint64(0); i < lim.validatorsPerCommittee; i++ {
			bits.SetBitAt(i, true)
		}
		return &qrysmpb.Attestation{
			AggregationBits: bits,
			Data:            HydrateAttestationData(&qrysmpb.AttestationData{}),
			Signatures:      sigs(lim.validatorsPerCommittee),
		}
	}
	mkIndexed := func() *qrysmpb.IndexedAttestation {
		idx := make([]uint64, lim.validatorsPerCommittee)
		for i := range idx {
			idx[i] = uint64(i)
		}
		return &qrysmpb.IndexedAttestation{
			AttestingIndices: idx,
			Data:             HydrateAttestationData(&qrysmpb.AttestationData{}),
			Signatures:       sigs(lim.validatorsPerCommittee),
		}
	}

	atts := make([]*qrysmpb.Attestation, lim.attestations)
	for i := range atts {
		atts[i] = mkAtt()
	}
	attesterSlashings := make([]*qrysmpb.AttesterSlashing, cfg.MaxAttesterSlashings)
	for i := range attesterSlashings {
		attesterSlashings[i] = &qrysmpb.AttesterSlashing{Attestation_1: mkIndexed(), Attestation_2: mkIndexed()}
	}
	proposerSlashings := make([]*qrysmpb.ProposerSlashing, cfg.MaxProposerSlashings)
	for i := range proposerSlashings {
		proposerSlashings[i] = &qrysmpb.ProposerSlashing{
			Header_1: HydrateSignedBeaconHeader(&qrysmpb.SignedBeaconBlockHeader{}),
			Header_2: HydrateSignedBeaconHeader(&qrysmpb.SignedBeaconBlockHeader{}),
		}
	}
	deposits := make([]*qrysmpb.Deposit, cfg.MaxDeposits)
	for i := range deposits {
		proof := make([][]byte, cfg.DepositContractTreeDepth+1)
		for j := range proof {
			proof[j] = make([]byte, fieldparams.RootLength)
		}
		deposits[i] = &qrysmpb.Deposit{
			Proof: proof,
			Data: &qrysmpb.Deposit_Data{
				PublicKey:           make([]byte, fieldparams.MLDSA87PubkeyLength),
				WithdrawalRecipient: make([]byte, fieldparams.WithdrawalRecipientLength),
				Amount:              cfg.MaxEffectiveBalance,
				Signature:           sig(),
				RandaoCommitment:    make([]byte, fieldparams.RandaoCommitmentLength),
			},
		}
	}
	exits := make([]*qrysmpb.SignedVoluntaryExit, cfg.MaxVoluntaryExits)
	for i := range exits {
		exits[i] = &qrysmpb.SignedVoluntaryExit{Exit: &qrysmpb.VoluntaryExit{}, Signature: sig()}
	}
	withdrawals := make([]*enginev1.Withdrawal, cfg.MaxWithdrawalsPerPayload)
	for i := range withdrawals {
		withdrawals[i] = &enginev1.Withdrawal{Address: make([]byte, fieldparams.FeeRecipientLength)}
	}

	body := &qrysmpb.BeaconBlockBodyZond{
		RandaoReveal: make([]byte, fieldparams.RandaoRevealLength),
		Graffiti:     make([]byte, fieldparams.RootLength),
		ExecutionData: &qrysmpb.ExecutionData{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		},
		ProposerSlashings: proposerSlashings,
		AttesterSlashings: attesterSlashings,
		Attestations:      atts,
		Deposits:          deposits,
		VoluntaryExits:    exits,
		SyncAggregate: &qrysmpb.SyncAggregate{
			SyncCommitteeBits:       make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignatures: sigs(cfg.SyncCommitteeSize),
		},
		ExecutionPayload: &enginev1.ExecutionPayloadZond{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			ExtraData:     make([]byte, 32),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			GasLimit:      qrlparams.MaxGasLimit,
			GasUsed:       gasUsed,
			Transactions:  txs,
			Withdrawals:   withdrawals,
		},
	}
	return &qrysmpb.SignedBeaconBlockZond{
		Block: &qrysmpb.BeaconBlockZond{
			ParentRoot: make([]byte, fieldparams.RootLength),
			StateRoot:  make([]byte, fieldparams.RootLength),
			Body:       body,
		},
		Signature: sig(),
	}
}

// TestMaxBlockSize_WorstCaseFitsP2PCaps is the gate between the consensus
// parameters and the network caps. It builds the largest block the beacon config
// and the SSZ types can express, with a full 20M-gas execution payload, and
// requires it to fit within GossipMaxSize and MaxChunkSize. If a future change to
// the gas limit, MaxAttestations, MaxValidatorsPerCommittee, SyncCommitteeSize
// or the slashing limits pushes a legal block past the caps, this test fails
// instead of the live network silently dropping un-gossipable blocks.
//
// The block must also round-trip through SSZ, which proves the generated
// bounds admit everything the config allows; the subtests prove the bounds are
// not looser than the config either.
func TestMaxBlockSize_WorstCaseFitsP2PCaps(t *testing.T) {
	netCfg := params.BeaconNetworkConfig()
	txs, gasUsed := fullGasLimitTransactions(t)

	block := worstCaseSignedBlock(t, configWorstCaseLimits(), txs, gasUsed)
	sszBytes, err := block.MarshalSSZ()
	require.NoError(t, err)

	decoded := &qrysmpb.SignedBeaconBlockZond{}
	require.NoError(t, decoded.UnmarshalSSZ(sszBytes), "worst-case block must round-trip through SSZ")

	size := uint64(len(sszBytes))
	t.Logf("Worst-case signed block SSZ size: %d bytes (%.2f MiB); GossipMaxSize %d, MaxChunkSize %d",
		size, float64(size)/(1<<20), netCfg.GossipMaxSize, netCfg.MaxChunkSize)
	require.Equal(t, true, size <= netCfg.GossipMaxSize,
		"worst-case block of %d bytes exceeds GossipMaxSize %d; it could never be gossiped", size, netCfg.GossipMaxSize)
	require.Equal(t, true, size <= netCfg.MaxChunkSize,
		"worst-case block of %d bytes exceeds MaxChunkSize %d; it could never be served by range/root requests", size, netCfg.MaxChunkSize)

	// Baseline for the subtests: the same block without transactions must
	// round-trip, so a failure below can only come from the nudged bound.
	base := worstCaseSignedBlock(t, configWorstCaseLimits(), nil, 0)
	baseEnc, err := base.MarshalSSZ()
	require.NoError(t, err)
	require.NoError(t, (&qrysmpb.SignedBeaconBlockZond{}).UnmarshalSSZ(baseEnc))

	// A block one step past either config bound must be rejected by the SSZ
	// types. This pins the proto ssz_max annotations to the config values.
	mustNotRoundTrip := func(t *testing.T, b *qrysmpb.SignedBeaconBlockZond, what string) {
		enc, err := b.MarshalSSZ()
		if err == nil {
			err = (&qrysmpb.SignedBeaconBlockZond{}).UnmarshalSSZ(enc)
		}
		require.NotNil(t, err, "%s round-tripped; the SSZ bound is looser than the config", what)
	}
	t.Run("one attestation over MaxAttestations", func(t *testing.T) {
		lim := configWorstCaseLimits()
		lim.attestations++
		mustNotRoundTrip(t, worstCaseSignedBlock(t, lim, nil, 0), "block with MaxAttestations+1 attestations")
	})
	t.Run("one signature over MaxValidatorsPerCommittee", func(t *testing.T) {
		lim := configWorstCaseLimits()
		lim.validatorsPerCommittee++
		mustNotRoundTrip(t, worstCaseSignedBlock(t, lim, nil, 0), "attestation with MaxValidatorsPerCommittee+1 signatures")
	})
}
