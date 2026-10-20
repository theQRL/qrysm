package interop

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/crypto/randao"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/require"
)

func TestNewPreminedGenesis_RejectsOversizedGeneratedValidatorCount(t *testing.T) {
	capacity, err := params.BeaconConfig().MaxActiveValidators()
	require.NoError(t, err)
	// A nil execution block also ensures rejection happens before constructing
	// the state, rather than after thousands of keys and onions are generated.
	st, err := NewPreminedGenesis(context.Background(), 0, capacity+1, version.Zond, nil)
	require.ErrorContains(t, fmt.Sprintf("genesis active validator count %d exceeds committee capacity %d", capacity+1, capacity), err)
	require.Equal(t, true, st == nil)
}

func TestNewPreminedGenesis_RejectsUnsafeCommitteeScaling(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.MaxCommitteesPerSlot = 2
	cfg.TargetCommitteeSize = cfg.MaxValidatorsPerCommittee
	params.OverrideBeaconConfig(cfg)
	capacity := uint64(cfg.SlotsPerEpoch) * cfg.MaxValidatorsPerCommittee
	st, err := NewPreminedGenesis(context.Background(), 0, capacity+1, version.Zond, nil)
	require.ErrorContains(t, fmt.Sprintf("genesis active validator count %d exceeds committee capacity %d", capacity+1, capacity), err)
	require.Equal(t, true, st == nil)
}

func TestNewPreminedGenesis_ActiveValidatorCapacity(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	if fieldparams.Preset == "minimal" {
		params.OverrideBeaconConfig(params.MinimalSpecConfig().Copy())
	}
	cfg := params.BeaconConfig()
	capacity, err := cfg.MaxActiveValidators()
	require.NoError(t, err)
	keys, _, err := DeterministicallyGenerateKeys(0, capacity+1)
	require.NoError(t, err)
	dds := make([]*qrysmpb.Deposit_Data, capacity+1)
	for i, key := range keys {
		dds[i] = shortOnionDepositData(t, key, cfg.MaxEffectiveBalance)
	}
	partial := shortOnionDepositData(t, keys[capacity], cfg.MinDepositAmount)
	withExtra := func(extra *qrysmpb.Deposit_Data) []*qrysmpb.Deposit_Data {
		return append(append([]*qrysmpb.Deposit_Data(nil), dds[:capacity]...), extra)
	}
	gb := GqrlTestnetGenesis(0, cfg).ToBlock()
	ctx := context.Background()

	for _, tc := range []struct {
		name           string
		deposits       []*qrysmpb.Deposit_Data
		wantValidators int
		wantErr        bool
	}{
		{name: "at capacity", deposits: dds[:capacity], wantValidators: int(capacity)},
		{name: "above capacity", deposits: dds, wantErr: true},
		{name: "extra partially funded validator", deposits: withExtra(partial), wantValidators: int(capacity + 1)},
		{name: "extra duplicate deposit", deposits: withExtra(dds[0]), wantValidators: int(capacity)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helpers.ClearCache()
			t.Cleanup(helpers.ClearCache)
			roots := make([][]byte, len(tc.deposits))
			for i, dd := range tc.deposits {
				root, err := dd.HashTreeRoot()
				require.NoError(t, err)
				roots[i] = root[:]
			}
			// NVals is deliberately zero: --deposit-json-file supplies its own
			// entries, so only the processed state tells us the active count.
			st, err := NewPreminedGenesis(ctx, 0, 0, version.Zond, gb, WithDepositData(tc.deposits, roots))
			if tc.wantErr {
				require.ErrorContains(t, fmt.Sprintf("genesis active validator count %d exceeds committee capacity %d", capacity+1, capacity), err)
				require.Equal(t, true, st == nil)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantValidators, st.NumValidators())
			active, err := helpers.ActiveValidatorCount(ctx, st, cfg.GenesisEpoch)
			require.NoError(t, err)
			require.Equal(t, capacity, active)
			_, err = st.MarshalSSZ()
			require.NoError(t, err)
		})
	}
}

// These deposits use valid signatures and short, valid RANDAO onions. The
// default million-layer onion is not a consensus requirement and would make
// a regression test at the mainnet validator capacity prohibitively expensive.
func shortOnionDepositData(t *testing.T, key ml_dsa_87.MLDSA87Key, amount uint64) *qrysmpb.Deposit_Data {
	t.Helper()
	commitment := randao.Commitment(key.Marshal(), 16)
	msg := &qrysmpb.DepositMessage{
		PublicKey:           key.PublicKey().Marshal(),
		WithdrawalRecipient: make([]byte, fieldparams.WithdrawalRecipientLength),
		Amount:              amount,
		RandaoCommitment:    commitment[:],
	}
	root, err := msg.HashTreeRoot()
	require.NoError(t, err)
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	require.NoError(t, err)
	signingRoot, err := (&qrysmpb.SigningData{ObjectRoot: root[:], Domain: domain}).HashTreeRoot()
	require.NoError(t, err)
	sig, err := key.Sign(signingRoot[:])
	require.NoError(t, err)
	return &qrysmpb.Deposit_Data{
		PublicKey:           msg.PublicKey,
		WithdrawalRecipient: msg.WithdrawalRecipient,
		Amount:              msg.Amount,
		RandaoCommitment:    msg.RandaoCommitment,
		Signature:           sig.Marshal(),
	}
}
