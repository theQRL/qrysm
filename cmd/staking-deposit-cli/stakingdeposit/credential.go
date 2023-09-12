package stakingdeposit

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/cyyber/qrysm/v4/beacon-chain/core/signing"
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/config"
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/misc"
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/stakingdeposit/keyhandling"
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/stakingdeposit/keyhandling/keyderivation"
	"github.com/cyyber/qrysm/v4/config/params"
	"github.com/cyyber/qrysm/v4/consensus-types/primitives"
	"github.com/cyyber/qrysm/v4/crypto/dilithium"
	"github.com/cyyber/qrysm/v4/crypto/hash"
	ethpbv2 "github.com/cyyber/qrysm/v4/proto/eth/v2"
	ethpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common"
)

type Credential struct {
	signingKeyPath           string
	withdrawalSeed           string
	signingSeed              string
	amount                   uint64
	chainSetting             *config.ChainSetting
	hexZondWithdrawalAddress string
}

func (c *Credential) ZondWithdrawalAddress() common.Address {
	if len(c.hexZondWithdrawalAddress) == 0 {
		return common.Address{}
	}
	return common.HexToAddress(c.hexZondWithdrawalAddress)
}

func (c *Credential) WithdrawalPK() []byte {
	binWithdrawalSeed := misc.StrSeedToBinSeed(c.withdrawalSeed)
	withdrawalKey, err := dilithium.SecretKeyFromBytes(binWithdrawalSeed[:])
	if err != nil {
		panic(fmt.Errorf("failed to generate dilithium key from withdrawalSeed %s", c.withdrawalSeed))
	}
	return withdrawalKey.PublicKey().Marshal()
}

func (c *Credential) WithdrawalPrefix() uint8 {
	withdrawalAddress := c.ZondWithdrawalAddress()
	if reflect.DeepEqual(withdrawalAddress, common.Address{}) {
		return params.BeaconConfig().ZondAddressWithdrawalPrefixByte
	}
	return params.BeaconConfig().DilithiumWithdrawalPrefixByte
}

func (c *Credential) WithdrawalType() byte {
	return c.WithdrawalPrefix()
}

func (c *Credential) WithdrawalCredentials() [32]byte {
	var withdrawalCredentials [32]byte

	withdrawalType := c.WithdrawalType()
	switch withdrawalType {
	case params.BeaconConfig().DilithiumWithdrawalPrefixByte:
		withdrawalCredentials[0] = params.BeaconConfig().DilithiumWithdrawalPrefixByte
		h := hash.Hash(c.WithdrawalPK())
		copy(withdrawalCredentials[1:], h[1:])
	case params.BeaconConfig().ZondAddressWithdrawalPrefixByte:
		zondWithdrawalAddress := c.ZondWithdrawalAddress()
		if reflect.DeepEqual(zondWithdrawalAddress, common.Address{}) {
			panic(fmt.Errorf("empty zond withdrawal address"))
		}
		withdrawalCredentials[0] = params.BeaconConfig().ZondAddressWithdrawalPrefixByte
		// 1 byte reserved for withdrawal prefix
		if common.AddressLength > len(withdrawalCredentials)-1 {
			panic(fmt.Errorf("address length %d is more than remaining length in withdrawal credentials %d",
				common.AddressLength, len(withdrawalCredentials)))
		}
		copy(withdrawalCredentials[len(withdrawalCredentials)-common.AddressLength:], zondWithdrawalAddress.Bytes())
	default:
		panic(fmt.Errorf("invalid withdrawal type %d", withdrawalType))
	}

	return withdrawalCredentials
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
	return c.signingSeed == hex.EncodeToString(seedBytes[:])
}

func (c *Credential) GetDilithiumToExecutionChange(validatorIndex uint64) *ethpbv2.SignedDilithiumToExecutionChange {
	if len(c.hexZondWithdrawalAddress) == 0 {
		panic("the execution address should not be empty")
	}

	binWithdrawalSeed, err := hex.DecodeString(c.withdrawalSeed)
	d, err := dilithium.SecretKeyFromBytes(binWithdrawalSeed)
	if err != nil {
		panic(fmt.Errorf("failed to generate secret Key from withdrawal seed %v", err))
	}

	message := &ethpbv2.DilithiumToExecutionChange{
		ValidatorIndex:      primitives.ValidatorIndex(validatorIndex),
		FromDilithiumPubkey: c.WithdrawalPK(),
		ToExecutionAddress:  c.ZondWithdrawalAddress().Bytes()}
	root, err := message.HashTreeRoot()
	if err != nil {
		panic(fmt.Errorf("failed to generate hash tree root for message %v", err))
	}

	domain, err := signing.ComputeDomain(
		params.BeaconConfig().DomainDilithiumToExecutionChange,
		c.chainSetting.GenesisForkVersion,    /*forkVersion*/
		c.chainSetting.GenesisValidatorsRoot, /*genesisValidatorsRoot*/
	)

	signingData := &ethpb.SigningData{
		ObjectRoot: root[:],
		Domain:     domain,
	}

	signingRoot, err := signingData.HashTreeRoot()
	if err != nil {
		panic(fmt.Errorf("failed to generate hash tree root for signingData %v", err))
	}
	signature := d.Sign(signingRoot[:])

	return &ethpbv2.SignedDilithiumToExecutionChange{
		Message:   message,
		Signature: signature.Marshal(),
	}
}

func (c *Credential) GetDilithiumToExecutionChangeData(validatorIndex uint64) *DilithiumToExecutionChangeData {
	signedDilithiumToExecutionChange := c.GetDilithiumToExecutionChange(validatorIndex)
	return NewDilithiumToExeuctionChangeData(signedDilithiumToExecutionChange, c.chainSetting)
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
