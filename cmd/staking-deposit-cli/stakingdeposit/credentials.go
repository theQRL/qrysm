package stakingdeposit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/config"
)

type Credentials struct {
	credentials []*Credential
}

func (c *Credentials) ExportKeystores(password, folder string) ([]string, error) {
	var filesAbsolutePath []string
	for _, credential := range c.credentials {
		fileAbsolutePath, err := credential.SaveSigningKeystore(password, folder)
		if err != nil {
			return nil, err
		}
		filesAbsolutePath = append(filesAbsolutePath, fileAbsolutePath)
	}
	return filesAbsolutePath, nil
}

func (c *Credentials) ExportDepositDataJSON(folder string) (string, error) {
	var depositDataList []*DepositData
	for _, credential := range c.credentials {
		depositData, err := NewDepositData(credential)
		if err != nil {
			return "", err
		}
		depositDataList = append(depositDataList, depositData)
	}

	fileFolder := filepath.Join(folder, fmt.Sprintf("deposit_data-%d.json", time.Now().Unix()))
	jsonDepositDataList, err := json.Marshal(depositDataList)
	if err != nil {
		return "", nil
	}

	if runtime.GOOS == "linux" {
		err = os.WriteFile(fileFolder, jsonDepositDataList, 0440)
	}
	return fileFolder, nil
}

func (c *Credentials) VerifyKeystores(keystoreFileFolders []string, password string) bool {
	for i, credential := range c.credentials {
		if !credential.VerifyKeystore(keystoreFileFolders[i], password) {
			return false
		}
	}
	return true
}

func (c *Credentials) ExportDilithiumToExecutionChangeJSON(folder string, validatorIndices []uint64) (string, error) {
	var dilithiumToExecutionChangeDataList []*DilithiumToExecutionChangeData
	for i, credential := range c.credentials {
		dilithiumToExecutionChangeData := credential.GetDilithiumToExecutionChangeData(validatorIndices[i])
		dilithiumToExecutionChangeDataList = append(dilithiumToExecutionChangeDataList, dilithiumToExecutionChangeData)
	}

	fileFolder := filepath.Join(folder, fmt.Sprintf("deposit_data-%d.json", time.Now().Unix()))
	jsonDepositDataList, err := json.Marshal(dilithiumToExecutionChangeDataList)
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "linux" {
		err = os.WriteFile(fileFolder, jsonDepositDataList, 0440)
		if err != nil {
			return "", err
		}
	}
	return fileFolder, err
}

func NewCredentialsFromSeed(seed string, numKeys uint64, amounts []uint64,
	chainSettings *config.ChainSetting, startIndex uint64, hexZondWithdrawalAddress string) (*Credentials, error) {
	credentials := &Credentials{
		credentials: make([]*Credential, numKeys),
	}
	for index := startIndex; index < startIndex+numKeys; index++ {
		c, err := NewCredential(seed, index, amounts[index], chainSettings, hexZondWithdrawalAddress)
		if err != nil {
			return nil, err
		}
		credentials.credentials[index] = c
	}
	return credentials, nil
}
