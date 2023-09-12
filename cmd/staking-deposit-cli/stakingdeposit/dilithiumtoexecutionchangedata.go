package stakingdeposit

import (
	"fmt"

	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/config"
	ethpbv2 "github.com/cyyber/qrysm/v4/proto/eth/v2"
)

type DilithiumToExecutionChangeMessage struct {
	ValidatorIndex      uint64
	FromDilithiumPubkey string
	ToExecutionAddress  string
}

type DilithiumToExecutionChangeMetaData struct {
	NetworkName           string
	GenesisValidatorsRoot string
	DepositCLIVersion     string
}

type DilithiumToExecutionChangeData struct {
	Message   *DilithiumToExecutionChangeMessage  `json:"message"`
	Signature string                              `json:"signature"`
	MetaData  *DilithiumToExecutionChangeMetaData `json:"metadata"`
}

func NewDilithiumToExeuctionChangeData(
	signedDilithiumToExecutionChange *ethpbv2.SignedDilithiumToExecutionChange,
	chainSetting *config.ChainSetting) *DilithiumToExecutionChangeData {
	return &DilithiumToExecutionChangeData{
		Message: &DilithiumToExecutionChangeMessage{
			ValidatorIndex:      uint64(signedDilithiumToExecutionChange.Message.ValidatorIndex),
			FromDilithiumPubkey: fmt.Sprintf("0x%x", signedDilithiumToExecutionChange.Message.FromDilithiumPubkey),
			ToExecutionAddress:  fmt.Sprintf("0x%x", signedDilithiumToExecutionChange.Message.ToExecutionAddress),
		},
		Signature: fmt.Sprintf("0x%x", signedDilithiumToExecutionChange.Signature),
		MetaData: &DilithiumToExecutionChangeMetaData{
			NetworkName:           chainSetting.Name,
			GenesisValidatorsRoot: fmt.Sprintf("0x%x", chainSetting.GenesisValidatorsRoot),
			DepositCLIVersion:     "", // TODO (cyyber): Assign cli version
		},
	}
}
