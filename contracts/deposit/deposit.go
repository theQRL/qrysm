// Package deposit contains useful functions for dealing
// with QRL deposit inputs.
package deposit

import (
	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// DepositInput for a given key. This input data can be used to when making a
// validator deposit. The input data includes a proof of possession field
// signed by the deposit key.
//
// Spec details about general deposit workflow:
//
//	To submit a deposit:
//
//	- Pack the validator's initialization parameters into deposit_data, a Deposit_Data SSZ object.
//	- Let amount be the amount in Shor to be deposited by the validator where MIN_DEPOSIT_AMOUNT <= amount <= MAX_EFFECTIVE_BALANCE.
//	- Set deposit_data.amount = amount.
//	- Let signature be the result of bls_sign of the signing_root(deposit_data) with domain=compute_domain(DOMAIN_DEPOSIT). (Deposits are valid regardless of fork version, compute_domain will default to zeroes there).
//	- Send a transaction on the QRL execution layer to DEPOSIT_CONTRACT_ADDRESS executing `deposit(pubkey: bytes[48], withdrawal_credentials: bytes[32], signature: bytes[96])` along with a deposit of amount Shor.
//
// See: https://github.com/ethereum/consensus-specs/blob/master/specs/validator/0_beacon-chain-validator.md#submit-deposit
func DepositInput(depositKey ml_dsa_87.MLDSA87Key, withdrawalAddr common.Address, amountInShor uint64, forkVersion []byte) (*qrysmpb.Deposit_Data, [32]byte, error) {
	depositMessage := &qrysmpb.DepositMessage{
		PublicKey:             depositKey.PublicKey().Marshal(),
		WithdrawalCredentials: WithdrawalCredentialsAddress(withdrawalAddr),
		Amount:                amountInShor,
	}

	sr, err := depositMessage.HashTreeRoot()
	if err != nil {
		return nil, [32]byte{}, err
	}

	domain, err := signing.ComputeDomain(
		params.BeaconConfig().DomainDeposit,
		forkVersion, /*forkVersion*/
		nil,         /*genesisValidatorsRoot*/
	)
	if err != nil {
		return nil, [32]byte{}, err
	}
	root, err := (&qrysmpb.SigningData{ObjectRoot: sr[:], Domain: domain}).HashTreeRoot()
	if err != nil {
		return nil, [32]byte{}, err
	}
	di := &qrysmpb.Deposit_Data{
		PublicKey:             depositMessage.PublicKey,
		WithdrawalCredentials: depositMessage.WithdrawalCredentials,
		Amount:                depositMessage.Amount,
		Signature:             depositKey.Sign(root[:]).Marshal(),
	}

	dr, err := di.HashTreeRoot()
	if err != nil {
		return nil, [32]byte{}, err
	}

	return di, dr, nil
}

// WithdrawalCredentialsAddress forms a 64 byte withdrawal credential containing
// the execution address.
func WithdrawalCredentialsAddress(addr common.Address) []byte {
	return addr.Bytes()
}

// VerifyDepositSignature verifies the correctness of Execution deposit ML-DSA-87 signature
func VerifyDepositSignature(dd *qrysmpb.Deposit_Data, domain []byte) error {
	ddCopy := qrysmpb.CopyDepositData(dd)
	publicKey, err := ml_dsa_87.PublicKeyFromBytes(ddCopy.PublicKey)
	if err != nil {
		return errors.Wrap(err, "could not convert bytes to public key")
	}
	sig, err := ml_dsa_87.SignatureFromBytes(ddCopy.Signature)
	if err != nil {
		return errors.Wrap(err, "could not convert bytes to signature")
	}
	di := &qrysmpb.DepositMessage{
		PublicKey:             ddCopy.PublicKey,
		WithdrawalCredentials: ddCopy.WithdrawalCredentials,
		Amount:                ddCopy.Amount,
	}
	root, err := di.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not get signing root")
	}
	signingData := &qrysmpb.SigningData{
		ObjectRoot: root[:],
		Domain:     domain,
	}
	ctrRoot, err := signingData.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not get container root")
	}
	if !sig.Verify(publicKey, ctrRoot[:]) {
		return signing.ErrSigFailedToVerify
	}
	return nil
}
