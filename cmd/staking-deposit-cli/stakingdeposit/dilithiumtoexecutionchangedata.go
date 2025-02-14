package stakingdeposit

import (
	"fmt"
	"strconv"

	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/config"
	zondpbv1 "github.com/theQRL/qrysm/proto/zond/v1"
)

type DilithiumToExecutionChangeMessage struct {
	ValidatorIndex      string `json:"validator_index"`
	FromDilithiumPubkey string `json:"from_dilithium_pubkey"`
	ToExecutionAddress  string `json:"to_execution_address"`
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

func NewDilithiumToExecutionChangeData(
	signedDilithiumToExecutionChange *zondpbv1.SignedDilithiumToExecutionChange,
	chainSetting *config.ChainSetting) *DilithiumToExecutionChangeData {
	return &DilithiumToExecutionChangeData{
		Message: &DilithiumToExecutionChangeMessage{
			ValidatorIndex:      strconv.FormatUint(uint64(signedDilithiumToExecutionChange.Message.ValidatorIndex), 10),
			FromDilithiumPubkey: fmt.Sprintf("0x%x", signedDilithiumToExecutionChange.Message.FromDilithiumPubkey),
			ToExecutionAddress:  fmt.Sprintf("Z%x", signedDilithiumToExecutionChange.Message.ToExecutionAddress),
		},
		Signature: fmt.Sprintf("0x%x", signedDilithiumToExecutionChange.Signature),
		MetaData: &DilithiumToExecutionChangeMetaData{
			NetworkName:           chainSetting.Name,
			GenesisValidatorsRoot: fmt.Sprintf("0x%x", chainSetting.GenesisValidatorsRoot),
			DepositCLIVersion:     "", // TODO (cyyber): Assign cli version
		},
	}
}
