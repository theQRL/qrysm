package stakingdeposit

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/config"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/misc"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/stakingdeposit/keyhandling"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/stakingdeposit/keyhandling/keyderivation"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/dilithium"
	"github.com/theQRL/qrysm/crypto/hash"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	zondpbv1 "github.com/theQRL/qrysm/proto/zond/v1"
)

type Credential struct {
	signingKeyPath           string
	withdrawalSeed           string
	signingSeed              string
	amount                   uint64
	chainSetting             *config.ChainSetting
	hexZondWithdrawalAddress string
}

func (c *Credential) ZondWithdrawalAddress() (common.Address, error) {
	if len(c.hexZondWithdrawalAddress) == 0 {
		return common.Address{}, nil
	}
	withdrawalAddress, err := common.NewAddressFromString(c.hexZondWithdrawalAddress)
	if err != nil {
		return common.Address{}, err
	}
	return withdrawalAddress, nil
}

func (c *Credential) WithdrawalPK() []byte {
	binWithdrawalSeed := misc.StrSeedToBinSeed(c.withdrawalSeed)
	withdrawalKey, err := dilithium.SecretKeyFromSeed(binWithdrawalSeed[:])
	if err != nil {
		panic(fmt.Errorf("failed to generate dilithium key from withdrawalSeed %s", c.withdrawalSeed))
	}
	return withdrawalKey.PublicKey().Marshal()
}

func (c *Credential) WithdrawalPrefix() (uint8, error) {
	withdrawalAddress, err := c.ZondWithdrawalAddress()
	if err != nil {
		return 0, err
	}
	if reflect.DeepEqual(withdrawalAddress, common.Address{}) {
		return params.BeaconConfig().ZondAddressWithdrawalPrefixByte, nil
	}
	return params.BeaconConfig().DilithiumWithdrawalPrefixByte, nil
}

func (c *Credential) WithdrawalType() (byte, error) {
	return c.WithdrawalPrefix()
}

func (c *Credential) WithdrawalCredentials() ([32]byte, error) {
	var withdrawalCredentials [32]byte

	withdrawalType, err := c.WithdrawalType()
	if err != nil {
		return [32]byte{}, err
	}

	switch withdrawalType {
	case params.BeaconConfig().DilithiumWithdrawalPrefixByte:
		withdrawalCredentials[0] = params.BeaconConfig().DilithiumWithdrawalPrefixByte
		h := hash.Hash(c.WithdrawalPK())
		copy(withdrawalCredentials[1:], h[1:])
	case params.BeaconConfig().ZondAddressWithdrawalPrefixByte:
		zondWithdrawalAddress, err := c.ZondWithdrawalAddress()
		if err != nil {
			return [32]byte{}, err
		}
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

	return withdrawalCredentials, nil
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
	return c.signingSeed == misc.EncodeHex(seedBytes[:])
}

func (c *Credential) GetDilithiumToExecutionChange(validatorIndex uint64) *zondpbv1.SignedDilithiumToExecutionChange {
	if len(c.hexZondWithdrawalAddress) == 0 {
		panic("the execution address should not be empty")
	}

	binWithdrawalSeed := misc.DecodeHex(c.withdrawalSeed)
	d, err := dilithium.SecretKeyFromSeed(binWithdrawalSeed)
	if err != nil {
		panic(fmt.Errorf("failed to generate secret Key from withdrawal seed %v", err))
	}

	execAddr, err := c.ZondWithdrawalAddress()
	if err != nil {
		panic(fmt.Errorf("failed to read withdrawal address %v", err))
	}

	message := &zondpbv1.DilithiumToExecutionChange{
		ValidatorIndex:      primitives.ValidatorIndex(validatorIndex),
		FromDilithiumPubkey: c.WithdrawalPK(),
		ToExecutionAddress:  execAddr.Bytes()}
	root, err := message.HashTreeRoot()
	if err != nil {
		panic(fmt.Errorf("failed to generate hash tree root for message %v", err))
	}

	domain, err := signing.ComputeDomain(
		params.BeaconConfig().DomainDilithiumToExecutionChange,
		c.chainSetting.GenesisForkVersion,    /*forkVersion*/
		c.chainSetting.GenesisValidatorsRoot, /*genesisValidatorsRoot*/
	)
	if err != nil {
		panic(fmt.Errorf("failed to compute domain %v", err))
	}

	signingData := &zondpb.SigningData{
		ObjectRoot: root[:],
		Domain:     domain,
	}

	signingRoot, err := signingData.HashTreeRoot()
	if err != nil {
		panic(fmt.Errorf("failed to generate hash tree root for signingData %v", err))
	}
	signature := d.Sign(signingRoot[:])

	return &zondpbv1.SignedDilithiumToExecutionChange{
		Message:   message,
		Signature: signature.Marshal(),
	}
}

func (c *Credential) GetDilithiumToExecutionChangeData(validatorIndex uint64) *DilithiumToExecutionChangeData {
	signedDilithiumToExecutionChange := c.GetDilithiumToExecutionChange(validatorIndex)
	return NewDilithiumToExecutionChangeData(signedDilithiumToExecutionChange, c.chainSetting)
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
