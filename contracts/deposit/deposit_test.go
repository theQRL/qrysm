package deposit_test

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/contracts/deposit"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestDepositInput_GeneratesPb(t *testing.T) {
	var seed [field_params.MLDSA87SeedLength]uint8
	_, err := rand.Read(seed[:])
	require.NoError(t, err)
	k1, err := ml_dsa_87.SecretKeyFromSeed(seed[:])
	require.NoError(t, err)

	withdrawalAddr, err := common.NewAddressFromString("Q00000000000000000000000000000000000000000000000000000000000000000000000000000000000000001234567890123456789012345678901234567890")
	require.NoError(t, err)

	result, _, err := deposit.DepositInput(k1, withdrawalAddr, 0, nil)
	require.NoError(t, err)
	assert.DeepEqual(t, k1.PublicKey().Marshal(), result.PublicKey)

	sig, err := ml_dsa_87.SignatureFromBytes(result.Signature)
	require.NoError(t, err)
	testData := &qrysmpb.DepositMessage{
		PublicKey:             result.PublicKey,
		WithdrawalCredentials: result.WithdrawalCredentials,
		Amount:                result.Amount,
	}
	sr, err := testData.HashTreeRoot()
	require.NoError(t, err)
	domain, err := signing.ComputeDomain(
		params.BeaconConfig().DomainDeposit,
		nil, /*forkVersion*/
		nil, /*genesisValidatorsRoot*/
	)
	require.NoError(t, err)
	root, err := (&qrysmpb.SigningData{ObjectRoot: sr[:], Domain: domain}).HashTreeRoot()
	require.NoError(t, err)
	assert.Equal(t, true, sig.Verify(k1.PublicKey(), root[:]))
}

func TestVerifyDepositSignature_ValidSig(t *testing.T) {
	deposits, _, err := util.DeterministicDepositsAndKeys(1)
	require.NoError(t, err)
	dep := deposits[0]
	domain, err := signing.ComputeDomain(
		params.BeaconConfig().DomainDeposit,
		params.BeaconConfig().GenesisForkVersion,
		params.BeaconConfig().ZeroHash[:],
	)
	require.NoError(t, err)
	err = deposit.VerifyDepositSignature(dep.Data, domain)
	require.NoError(t, err)
}

func TestVerifyDepositSignature_InvalidSig(t *testing.T) {
	deposits, _, err := util.DeterministicDepositsAndKeys(1)
	require.NoError(t, err)
	dep := deposits[0]
	domain, err := signing.ComputeDomain(
		params.BeaconConfig().DomainDeposit,
		params.BeaconConfig().GenesisForkVersion,
		params.BeaconConfig().ZeroHash[:],
	)
	require.NoError(t, err)
	dep.Data.Signature = dep.Data.Signature[1:]
	err = deposit.VerifyDepositSignature(dep.Data, domain)
	if err == nil {
		t.Fatal("Deposit Verification succeeds with a invalid signature")
	}
}

func TestWithdrawalCredentialsAddress(t *testing.T) {
	type tc struct {
		name    string
		addrHex string
		wantHex string
	}
	tests := []tc{
		{
			name:    "zero address",
			addrHex: "Q00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			name:    "leading zeros preserved",
			addrHex: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000102030405060708090a0b0c0d0e0f10111213",
		},
		{
			name:    "all 0xff",
			addrHex: "Qffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.wantHex = "0x" + strings.TrimPrefix(strings.ToLower(tc.addrHex), "q")
			addr, err := common.NewAddressFromString(tc.addrHex)
			require.NoError(t, err)
			got := deposit.WithdrawalCredentialsAddress(addr)
			gotHex := "0x" + hex.EncodeToString(got)
			require.Equal(t, tc.wantHex, gotHex)
		})
	}

}
