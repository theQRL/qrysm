package stakingdeposit

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/config"
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/misc"
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/stakingdeposit/keyhandling"
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/stakingdeposit/keyhandling/keyderivation"
)

type Credential struct {
	signingKeyPath           string
	withdrawalSeed           string
	signingSeed              string
	amount                   uint64
	chainSetting             *config.ChainSetting
	hexZondWithdrawalAddress string
}

func (c *Credential) signingKeystore(password string) (*keyhandling.Keystore, error) {
	seed := misc.StrSeedToBinSeed(c.signingSeed)
	return keyhandling.Encrypt(seed, password, c.signingKeyPath, nil, nil)
}

func (c *Credential) SaveSigningKeystore(password string, folder string) (string, error) {
	keystore, err := c.signingKeystore(password)
	if err != nil {
		return "", err
	}
	fileFolder := filepath.Join(folder, fmt.Sprintf("keystore-%s-%d.json",
		strings.Replace(keystore.Path, "/", "_", -1),
		time.Now().Unix()))
	return fileFolder, keystore.Save(fileFolder)
}

func (c *Credential) VerifyKeystore(keystoreFileFolder, password string) bool {
	savedKeystore := keyhandling.NewKeystoreFromFile(keystoreFileFolder)
	seedBytes := savedKeystore.Decrypt(password)
	return c.signingSeed == hex.EncodeToString(seedBytes)
}

func NewCredential(seed string, index, amount uint64,
	chainSetting *config.ChainSetting, hexZondWithdrawalAddress string) (*Credential, error) {
	purpose := "12381" // TODO (cyyber): Purpose code to be decided later
	coinType := "238"  // TODO (cyyber): coinType to be decided later
	account := strconv.FormatUint(index, 10)
	withdrawalKeyPath := fmt.Sprintf("m/%s/%s/%s/0", purpose, coinType, account)

	signingKeyPath := fmt.Sprintf("%s/0", withdrawalKeyPath)
	withdrawalSeed, err := keyderivation.SeedAndPathToSeed(seed, withdrawalKeyPath)
	if err != nil {
		return nil, err
	}
	signingSeed, err := keyderivation.SeedAndPathToSeed(seed, signingKeyPath)
	if err != nil {
		return nil, err
	}
	return &Credential{
		signingKeyPath:           signingKeyPath,
		withdrawalSeed:           withdrawalSeed,
		signingSeed:              signingSeed,
		amount:                   amount,
		chainSetting:             chainSetting,
		hexZondWithdrawalAddress: hexZondWithdrawalAddress,
	}, nil
}
