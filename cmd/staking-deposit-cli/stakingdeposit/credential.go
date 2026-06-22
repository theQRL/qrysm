package stakingdeposit

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/config"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/misc"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/stakingdeposit/keyhandling"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/stakingdeposit/keyhandling/keyderivation"
)

type Credential struct {
	signingKeyPath    string
	signingSeed       string
	amount            uint64
	chainSetting      *config.ChainSetting
	withdrawalAddress common.Address
}

func (c *Credential) signingKeystore(password string, lightKDF bool) (*keyhandling.Keystore, error) {
	seed := misc.StrSeedToBinSeed(c.signingSeed)
	return keyhandling.Encrypt(seed, password, c.signingKeyPath, lightKDF, nil, nil)
}

func (c *Credential) SaveSigningKeystore(password string, folder string, lightKDF bool) (string, error) {
	keystore, err := c.signingKeystore(password, lightKDF)
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
	return c.signingSeed == misc.EncodeHex(seedBytes[:])
}

func NewCredential(seed string, index, amount uint64,
	chainSetting *config.ChainSetting, withdrawalAddress common.Address) (*Credential, error) {
	purpose := "12381" // TODO (cyyber): Purpose code to be decided later
	coinType := "238"  // TODO (cyyber): coinType to be decided later
	account := strconv.FormatUint(index, 10)
	signingKeyPath := fmt.Sprintf("m/%s/%s/%s/0", purpose, coinType, account)
	signingSeed, err := keyderivation.SeedAndPathToSeed(seed, signingKeyPath)
	if err != nil {
		return nil, err
	}
	return &Credential{
		signingKeyPath:    signingKeyPath,
		signingSeed:       signingSeed,
		amount:            amount,
		chainSetting:      chainSetting,
		withdrawalAddress: withdrawalAddress,
	}, nil
}
