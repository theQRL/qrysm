package stakingdeposit

import (
	"github.com/theQRL/qrysm/v4/cmd/staking-deposit-cli/misc"
	"github.com/theQRL/qrysm/v4/contracts/deposit"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
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

	depositMessage := &zondpb.DepositMessage{
		PublicKey:             depositKey.PublicKey().Marshal(),
		WithdrawalCredentials: deposit.WithdrawalCredentialsHash(withdrawalKey),
		Amount:                c.amount,
	}

	messageRoot, err := depositMessage.HashTreeRoot()
	if err != nil {
		return nil, err
	}

	d := &DepositData{
		PubKey:                misc.EncodeHex(depositMessage.PublicKey),
		WithdrawalCredentials: misc.EncodeHex(depositMessage.WithdrawalCredentials),
		Amount:                c.amount,
		Signature:             misc.EncodeHex(depositData.Signature),
		MessageRoot:           misc.EncodeHex(messageRoot[:]),
		DepositDataRoot:       misc.EncodeHex(dataRoot[:]),
		ForkVersion:           misc.EncodeHex(c.chainSetting.GenesisForkVersion),
		NetworkName:           c.chainSetting.Name,
		CLIVersion:            "", // TODO: (cyyber) get CLI Version
	}
	return d, nil
}
