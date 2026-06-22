package stakingdeposit

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/config"
	"github.com/theQRL/qrysm/monitoring/progress"
)

type Credentials struct {
	credentials []*Credential
}

func (c *Credentials) ExportKeystores(password, folder string, lightKDF bool) ([]string, error) {
	bar := progress.InitializeProgressBar(len(c.credentials), "Generating keystores...")
	var filesAbsolutePath []string
	for _, credential := range c.credentials {
		fileAbsolutePath, err := credential.SaveSigningKeystore(password, folder, lightKDF)
		if err != nil {
			return nil, err
		}
		filesAbsolutePath = append(filesAbsolutePath, fileAbsolutePath)
		if err := bar.Add(1); err != nil {
			return nil, err
		}
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
		return "", err
	}

	f, err := os.Create(fileFolder)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Println(err)
		}
	}()

	if _, err := f.Write(jsonDepositDataList); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(fileFolder, 0440); err != nil {
			return "", err
		}
	}
	return fileFolder, err
}

func (c *Credentials) VerifyKeystores(keystoreFileFolders []string, password string) bool {
	bar := progress.InitializeProgressBar(len(c.credentials), "Verifying keystores...")
	for i, credential := range c.credentials {
		if !credential.VerifyKeystore(keystoreFileFolders[i], password) {
			return false
		}
		if err := bar.Add(1); err != nil {
			return false
		}
	}
	return true
}

func NewCredentialsFromSeed(seed string, numKeys uint64, amounts []uint64,
	chainSettings *config.ChainSetting, startIndex uint64, withdrawalAddr common.Address) (*Credentials, error) {
	credentials := &Credentials{
		credentials: make([]*Credential, numKeys),
	}
	for index := startIndex; index < startIndex+numKeys; index++ {
		c, err := NewCredential(seed, index, amounts[index-startIndex], chainSettings, withdrawalAddr)
		if err != nil {
			return nil, err
		}
		credentials.credentials[index-startIndex] = c
	}
	return credentials, nil
}
