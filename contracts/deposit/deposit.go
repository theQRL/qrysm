// Package deposit contains useful functions for dealing
// with QRL deposit inputs.
package deposit

import (
	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/crypto/randao"
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
//	- Set deposit_data.randao_commitment to the top layer of the validator's RANDAO hash onion (see crypto/randao).
//	- Send a transaction on the QRL execution layer to DEPOSIT_CONTRACT_ADDRESS executing `deposit(pubkey: bytes[2592], withdrawal_recipient: bytes[64], randao_commitment: bytes[32], signature: bytes[4627])` along with a deposit of amount Shor.
//
// The RANDAO commitment is derived from the deposit key's seed with the
// default onion length. Use DepositInputWithRandaoCommitment to supply one.
//
// See: https://github.com/ethereum/consensus-specs/blob/master/specs/validator/0_beacon-chain-validator.md#submit-deposit
func DepositInput(depositKey ml_dsa_87.MLDSA87Key, withdrawalAddr common.Address, amountInShor uint64, forkVersion []byte) (*qrysmpb.Deposit_Data, [32]byte, error) {
	commitment := randao.Commitment(depositKey.Marshal(), randao.DefaultLayers)
	return DepositInputWithRandaoCommitment(depositKey, withdrawalAddr, amountInShor, forkVersion, commitment)
}

// DepositInputWithRandaoCommitment is DepositInput with an explicit RANDAO
// hash-onion commitment.
func DepositInputWithRandaoCommitment(
	depositKey ml_dsa_87.MLDSA87Key,
	withdrawalAddr common.Address,
	amountInShor uint64,
	forkVersion []byte,
	randaoCommitment [fieldparams.RandaoCommitmentLength]byte,
) (*qrysmpb.Deposit_Data, [32]byte, error) {
	depositMessage := &qrysmpb.DepositMessage{
		PublicKey:           depositKey.PublicKey().Marshal(),
		WithdrawalRecipient: withdrawalAddr.Bytes(),
		Amount:              amountInShor,
		RandaoCommitment:    randaoCommitment[:],
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
	sig, err := depositKey.Sign(root[:])
	if err != nil {
		return nil, [32]byte{}, err
	}
	di := &qrysmpb.Deposit_Data{
		PublicKey:           depositMessage.PublicKey,
		WithdrawalRecipient: depositMessage.WithdrawalRecipient,
		Amount:              depositMessage.Amount,
		RandaoCommitment:    depositMessage.RandaoCommitment,
		Signature:           sig.Marshal(),
	}

	dr, err := di.HashTreeRoot()
	if err != nil {
		return nil, [32]byte{}, err
	}

	return di, dr, nil
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
		PublicKey:           ddCopy.PublicKey,
		WithdrawalRecipient: ddCopy.WithdrawalRecipient,
		Amount:              ddCopy.Amount,
		RandaoCommitment:    ddCopy.RandaoCommitment,
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
