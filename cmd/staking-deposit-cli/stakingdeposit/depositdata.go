package stakingdeposit

import (
	"encoding/hex"

	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/misc"
	"github.com/cyyber/qrysm/v4/contracts/deposit"
	"github.com/cyyber/qrysm/v4/crypto/dilithium"
	ethpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
)

type DepositData struct {
	PubKey                string `json:"pubkey"`
	Amount                uint64 `json:"amount"`
	WithdrawalCredentials string `json:"withdrawal_credentials"`
	DepositDataRoot       string `json:"deposit_data_root"`
	Signature             string `json:"signature"`

	MessageRoot string `json:"message_root"`
	ForkVersion string `json:"fork_version"`
	NetworkName string `json:"network_name"`
	CLIVersion  string `json:"deposit_cli_version"`
}

func NewDepositData(c *Credential) (*DepositData, error) {
	binSigningSeed := misc.StrSeedToBinSeed(c.signingSeed)
	depositKey, err := dilithium.SecretKeyFromBytes(binSigningSeed[:])
	if err != nil {
		return nil, err
	}

	binWithdrawalSeed := misc.StrSeedToBinSeed(c.withdrawalSeed)
	withdrawalKey, err := dilithium.SecretKeyFromBytes(binWithdrawalSeed[:])
	if err != nil {
		return nil, err
	}

	depositData, dataRoot, err := deposit.DepositInput(depositKey, withdrawalKey, c.amount, c.chainSetting.GenesisForkVersion)
	if err != nil {
		return nil, err
	}

	depositMessage := &ethpb.DepositMessage{
		PublicKey:             depositKey.PublicKey().Marshal(),
		WithdrawalCredentials: deposit.WithdrawalCredentialsHash(withdrawalKey),
		Amount:                c.amount,
	}

	messageRoot, err := depositMessage.HashTreeRoot()
	if err != nil {
		return nil, err
	}

	d := &DepositData{
		PubKey:                hex.EncodeToString(depositMessage.PublicKey),
		WithdrawalCredentials: hex.EncodeToString(depositMessage.WithdrawalCredentials),
		Amount:                c.amount,
		Signature:             hex.EncodeToString(depositData.Signature),
		MessageRoot:           hex.EncodeToString(messageRoot[:]),
		DepositDataRoot:       hex.EncodeToString(dataRoot[:]),
		ForkVersion:           hex.EncodeToString(c.chainSetting.GenesisForkVersion),
		NetworkName:           c.chainSetting.Name,
		CLIVersion:            "", // TODO: (cyyber) get CLI Version
	}
	return d, nil
}
