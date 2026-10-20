package blocks_test

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	v "github.com/theQRL/qrysm/beacon-chain/core/validators"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

// probeOutOfRangeIndex is far beyond any registry built by these probes.
const probeOutOfRangeIndex = primitives.ValidatorIndex(1 << 40)

// noPanic runs fn and reports a panic as a test failure instead of aborting
// the whole package, so every case still reports.
func noPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: %v", r)
		}
	}()
	fn()
}

func probeProposerSlashing(idx primitives.ValidatorIndex) *qrysmpb.ProposerSlashing {
	header := func(body byte) *qrysmpb.SignedBeaconBlockHeader {
		return &qrysmpb.SignedBeaconBlockHeader{
			Header: &qrysmpb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: idx,
				ParentRoot:    make([]byte, 32),
				StateRoot:     make([]byte, 32),
				BodyRoot:      bytesutil.PadTo([]byte{body}, 32),
			},
			Signature: make([]byte, field_params.MLDSA87SignatureLength),
		}
	}
	return &qrysmpb.ProposerSlashing{Header_1: header(1), Header_2: header(2)}
}

// deposit.go:229 - verifyDeposit hands the block's proof straight to the Merkle
// verifier. Malformed proof shapes must be rejected, not indexed.
func TestNoPanic_DepositProofShapes(t *testing.T) {
	st, _ := util.DeterministicGenesisStateZond(t, 8)
	depth := params.BeaconConfig().DepositContractTreeDepth
	data := &qrysmpb.Deposit_Data{
		PublicKey:           make([]byte, field_params.MLDSA87PubkeyLength),
		WithdrawalRecipient: make([]byte, 64),
		Amount:              params.BeaconConfig().MaxEffectiveBalance,
		Signature:           make([]byte, field_params.MLDSA87SignatureLength),
		RandaoCommitment:    make([]byte, 32),
	}
	oneByteEach := make([][]byte, depth+1)
	for i := range oneByteEach {
		oneByteEach[i] = []byte{byte(i)}
	}
	tooMany := make([][]byte, depth+2)
	for i := range tooMany {
		tooMany[i] = make([]byte, 32)
	}
	cases := []struct {
		name  string
		proof [][]byte
	}{
		{"nil proof", nil},
		{"empty proof", [][]byte{}},
		{"single short element", [][]byte{{0x01}}},
		{"depth+1 nil elements", make([][]byte, depth+1)},
		{"depth+1 one-byte elements", oneByteEach},
		{"depth+2 elements", tooMany},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			noPanic(t, func() {
				_, _, err := blocks.ProcessDeposit(st, &qrysmpb.Deposit{Proof: tc.proof, Data: data}, true)
				require.ErrorContains(t, "did not verify", err)
			})
		})
	}
	noPanic(t, func() {
		_, _, err := blocks.ProcessDeposit(st, nil, true)
		require.ErrorContains(t, "nil deposit", err)
		_, _, err = blocks.ProcessDeposit(st, &qrysmpb.Deposit{}, true)
		require.ErrorContains(t, "nil deposit", err)
	})
}

// randao.go:77 - VerifyRandaoReveal converts the proposer's stored commitment to
// a fixed 32-byte array. A short or long stored commitment must fail, not panic.
func TestNoPanic_RandaoRevealCommitmentLengths(t *testing.T) {
	ctx := context.Background()
	st, _ := util.DeterministicGenesisStateZond(t, 8)
	require.NoError(t, st.SetSlot(1))
	idx, err := helpers.BeaconProposerIndex(ctx, st)
	require.NoError(t, err)
	for _, n := range []int{0, 5, 40} {
		val, err := st.ValidatorAtIndex(idx)
		require.NoError(t, err)
		val.RandaoCommitment = make([]byte, n)
		require.NoError(t, st.UpdateValidatorAtIndex(idx, val))
		noPanic(t, func() {
			err := blocks.VerifyRandaoReveal(ctx, st, [field_params.RandaoRevealLength]byte{1})
			require.ErrorIs(t, err, blocks.ErrInvalidRandaoReveal, "commitment length %d", n)
			_, err = blocks.ProcessRandaoNoVerify(ctx, st, [field_params.RandaoRevealLength]byte{1})
			require.NoError(t, err, "commitment length %d", n)
		})
	}
}

// attester_slashing.go:115 - every intersecting index is looked up in the registry.
func TestNoPanic_AttesterSlashingIndexOutOfRange(t *testing.T) {
	ctx := context.Background()
	st, _ := util.DeterministicGenesisStateZond(t, 8)
	mk := func(blockRoot byte) *qrysmpb.IndexedAttestation {
		return &qrysmpb.IndexedAttestation{
			AttestingIndices: []uint64{uint64(probeOutOfRangeIndex)},
			Data:             util.HydrateAttestationData(&qrysmpb.AttestationData{BeaconBlockRoot: bytesutil.PadTo([]byte{blockRoot}, 32)}),
			Signatures:       [][]byte{make([]byte, field_params.MLDSA87SignatureLength)},
		}
	}
	sl := &qrysmpb.AttesterSlashing{Attestation_1: mk(1), Attestation_2: mk(2)}
	noPanic(t, func() {
		_, err := blocks.ProcessAttesterSlashingNoVerify(ctx, st, sl, v.SlashValidator)
		require.ErrorContains(t, "does not exist", err)
		_, err = blocks.ProcessAttesterSlashing(ctx, st, sl, v.SlashValidator)
		require.ErrorContains(t, "out of range", err)
	})
}

// exit.go:63 - the exit's validator index is looked up in the registry.
func TestNoPanic_VoluntaryExitIndexOutOfRange(t *testing.T) {
	st, _ := util.DeterministicGenesisStateZond(t, 8)
	exits := []*qrysmpb.SignedVoluntaryExit{{
		Exit:      &qrysmpb.VoluntaryExit{ValidatorIndex: probeOutOfRangeIndex},
		Signature: make([]byte, field_params.MLDSA87SignatureLength),
	}}
	noPanic(t, func() {
		_, err := blocks.ProcessVoluntaryExits(context.Background(), st, exits)
		require.ErrorContains(t, "does not exist", err)
	})
}

// proposer_slashing.go:131 - the header's proposer index goes to the slash
// callback; the verify path bounds it at line 157, the no-verify path does not.
func TestNoPanic_ProposerSlashingIndexOutOfRange(t *testing.T) {
	ctx := context.Background()
	st, _ := util.DeterministicGenesisStateZond(t, 8)
	require.NoError(t, st.SetSlot(1))
	sl := probeProposerSlashing(probeOutOfRangeIndex)
	noPanic(t, func() {
		_, err := blocks.ProcessProposerSlashingNoVerify(ctx, st, sl, v.SlashValidator)
		require.ErrorContains(t, "does not exist", err)
		_, err = blocks.ProcessProposerSlashing(ctx, st, sl, v.SlashValidator)
		require.ErrorContains(t, "does not exist", err)
	})
}

// validators/validator.go:157 (reached from both slashing processors) - the
// slashings vector is indexed by epoch with no bounds check. SSZ fixes that
// vector's size, so only a proto- or DB-built state can be short. Panicked before the guard.
func TestNoPanic_SlashValidatorShortSlashingsVector(t *testing.T) {
	ctx := context.Background()
	st, _ := util.DeterministicGenesisStateZond(t, 8)
	require.NoError(t, st.SetSlashings([]uint64{0}))
	require.NoError(t, st.SetSlot(params.BeaconConfig().SlotsPerEpoch.Mul(3))) // epoch 3 indexes past a 1-entry vector
	noPanic(t, func() {
		_, err := blocks.ProcessProposerSlashingNoVerify(ctx, st, probeProposerSlashing(0), v.SlashValidator)
		require.ErrorContains(t, "slashings index 3 out of range", err)
	})
}

// header.go:114 - ProcessBlockHeaderNoVerify dereferences the state's latest
// block header. A nil header must error, not panic. Panicked before the guard.
func TestNoPanic_BlockHeaderNilLatestBlockHeader(t *testing.T) {
	ctx := context.Background()
	st, _ := util.DeterministicGenesisStateZond(t, 8)
	require.NoError(t, st.SetSlot(1))
	idx, err := helpers.BeaconProposerIndex(ctx, st)
	require.NoError(t, err)
	require.NoError(t, st.SetLatestBlockHeader(nil))
	require.Equal(t, true, st.LatestBlockHeader() == nil, "setter did not store a nil header")
	noPanic(t, func() {
		_, err := blocks.ProcessBlockHeaderNoVerify(ctx, st, 1, idx, make([]byte, 32), make([]byte, 32))
		require.ErrorContains(t, "nil latest block header", err)
	})
}

// Exported helpers that other packages call with caller-supplied objects must
// reject nil arguments and degenerate states instead of dereferencing them.
func TestNoPanic_ExportedHelpersRejectNilArguments(t *testing.T) {
	ctx := context.Background()
	st, _ := util.DeterministicGenesisStateZond(t, 8)

	// proposer_slashing.go: called from gossip, block validation and the REST pool.
	noPanic(t, func() {
		require.ErrorContains(t, "nil proposer slashing", blocks.VerifyProposerSlashing(st, nil))
		require.ErrorContains(t, "nil header", blocks.VerifyProposerSlashing(st, &qrysmpb.ProposerSlashing{}))
	})
	// signature.go: called by the slasher.
	noPanic(t, func() {
		require.ErrorContains(t, "nil block header", blocks.VerifyBlockHeaderSignature(st, nil))
		require.ErrorContains(t, "nil block header", blocks.VerifyBlockHeaderSignature(st, &qrysmpb.SignedBeaconBlockHeader{}))
	})
	// execution_data.go: a nil vote must not be appended to the state.
	noPanic(t, func() {
		before := len(st.ExecutionDataVotes())
		_, err := blocks.ProcessExecutionDataInBlock(ctx, st, nil)
		require.ErrorContains(t, "nil execution data", err)
		require.Equal(t, before, len(st.ExecutionDataVotes()))
	})
	// withdrawals.go: nil payload, and an empty registry that used to divide by
	// zero at the sweep update.
	emptyPayload, err := consensusblocks.WrappedExecutionPayloadZond(&enginev1.ExecutionPayloadZond{}, 0)
	require.NoError(t, err)
	noPanic(t, func() {
		_, err := blocks.ProcessWithdrawals(st, nil)
		require.ErrorContains(t, "nil execution data", err)
		empty, err := util.NewBeaconStateZond()
		require.NoError(t, err)
		_, err = blocks.ProcessWithdrawals(empty, emptyPayload)
		require.ErrorContains(t, "no validators", err)
	})
}
