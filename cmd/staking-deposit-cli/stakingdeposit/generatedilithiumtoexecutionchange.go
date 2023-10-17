package stakingdeposit

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/v4/cmd/staking-deposit-cli/config"
	"github.com/theQRL/qrysm/v4/cmd/staking-deposit-cli/misc"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	ethpbv2 "github.com/theQRL/qrysm/v4/proto/eth/v2"
	ethpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

func GenerateDilithiumToExecutionChange(dilithiumExecutionChangesFolder string,
	chain,
	seed string,
	validatorStartIndex uint64,
	validatorIndices []uint64,
	dilithiumWithdrawalCredentialsList []string,
	executionAddress string,
	devnetChainSetting string) {
	dilithiumExecutionChangesFolder = filepath.Join(dilithiumExecutionChangesFolder, defaultDilithiumToExecutionChangesFolderName)
	if _, err := os.Stat(dilithiumExecutionChangesFolder); os.IsNotExist(err) {
		err := os.MkdirAll(dilithiumExecutionChangesFolder, 0775)
		if err != nil {
			panic(fmt.Errorf("cannot create folder. reason: %v", err))
		}
	}
	chainSettings, ok := config.GetConfig().ChainSettings[chain]
	if !ok {
		panic(fmt.Errorf("cannot find chain settings for %s", chain))
	}
	if len(devnetChainSetting) != 0 {
		devnetChainSettingMap := make(map[string]string)
		err := json.Unmarshal([]byte(devnetChainSetting), &devnetChainSettingMap)
		if err != nil {
			panic(fmt.Errorf("failed to unmarshal devnetChainSetting %s | reason %v", devnetChainSetting, err))
		}
		networkName, ok := devnetChainSettingMap["network_name"]
		if !ok {
			panic("network_name not found in devnetChainSetting passed as argument")
		}
		genesisForkVersion, ok := devnetChainSettingMap["genesis_fork_version"]
		if !ok {
			panic("genesis_fork_version not found in devnetChainSetting passed as argument")
		}
		genesisValidatorRoot, ok := devnetChainSettingMap["genesis_validator_root"]
		if !ok {
			panic("genesis_validator_root not found in devnetChainSetting passed as argument")
		}
		chainSettings = &config.ChainSetting{
			Name:                  networkName,
			GenesisForkVersion:    config.ToHex(genesisForkVersion),
			GenesisValidatorsRoot: config.ToHex(genesisValidatorRoot),
		}
	}

	numValidators := uint64(len(validatorIndices))
	if numValidators != uint64(len(dilithiumWithdrawalCredentialsList)) {
		panic(fmt.Errorf("length of validatorIndices %d should be same as dilithiumWithdrawalCredentialsList %d",
			numValidators, len(dilithiumWithdrawalCredentialsList)))
	}

	amounts := make([]uint64, numValidators)
	for i := uint64(0); i < numValidators; i++ {
		amounts[i] = params.BeaconConfig().MaxEffectiveBalance
	}

	credentials, err := NewCredentialsFromSeed(seed, numValidators, amounts, chainSettings, validatorStartIndex, executionAddress)
	if err != nil {
		panic(fmt.Errorf("new credentials from mnemonic failed. reason: %v", err))
	}

	for i, credential := range credentials.credentials {
		if !ValidateDilithiumWithdrawalCredentialsMatching(dilithiumWithdrawalCredentialsList[i], credential) {
			panic("dilithium withdrawal credential not matching")
		}
	}

	dtecFile, err := credentials.ExportDilithiumToExecutionChangeJSON(dilithiumExecutionChangesFolder, validatorIndices)
	if err != nil {
		panic(fmt.Errorf("error in ExportDilithiumToExecutionChangeJSON %v", err))
	}
	if !VerifyDilithiumToExecutionChangeJSON(dtecFile, credentials, validatorIndices, executionAddress, chainSettings) {
		panic("failed to verify the dilithium to execution change json file")
	}
}

func VerifyDilithiumToExecutionChangeJSON(fileFolder string,
	credentials *Credentials,
	inputValidatorIndices []uint64,
	inputExecutionAddress string,
	chainSetting *config.ChainSetting) bool {
	data, err := os.ReadFile(fileFolder)
	if err != nil {
		panic(fmt.Errorf("failed to read file %s | reason %v", fileFolder, err))
	}
	var dilithiumToExecutionChangeDataList []*DilithiumToExecutionChangeData
	err = json.Unmarshal(data, &dilithiumToExecutionChangeDataList)
	if err != nil {
		panic(fmt.Errorf("failed to unmarshal file %s | reason %v", fileFolder, err))
	}

	for i, dilithiumToExecutionChange := range dilithiumToExecutionChangeDataList {
		if !ValidateDilithiumToExecutionChange(dilithiumToExecutionChange,
			credentials.credentials[i], inputValidatorIndices[i], inputExecutionAddress, chainSetting) {
			return false
		}
	}

	return true
}

func ValidateDilithiumToExecutionChange(dilithiumToExecutionChange *DilithiumToExecutionChangeData,
	credential *Credential, inputValidatorIndex uint64, inputExecutionAddress string, chainSetting *config.ChainSetting) bool {
	validatorIndex := dilithiumToExecutionChange.Message.ValidatorIndex
	fromDilithiumPubkey, err := dilithium.PublicKeyFromBytes(misc.DecodeHex(dilithiumToExecutionChange.Message.FromDilithiumPubkey))
	if err != nil {
		panic(fmt.Errorf("failed to convert %s to dilithium public key | reason %v",
			dilithiumToExecutionChange.Message.FromDilithiumPubkey, err))
	}
	toExecutionAddress := misc.DecodeHex(dilithiumToExecutionChange.Message.ToExecutionAddress)
	signature, err := dilithium.SignatureFromBytes(misc.DecodeHex(dilithiumToExecutionChange.Signature))
	if err != nil {
		panic(fmt.Errorf("failed to convert %s to dilithium signature | reason %v",
			dilithiumToExecutionChange.Signature, err))
	}
	genesisValidatorsRoot := misc.DecodeHex(dilithiumToExecutionChange.MetaData.GenesisValidatorsRoot)

	uintValidatorIndex, err := strconv.ParseUint(validatorIndex, 10, 64)
	if err != nil {
		panic(fmt.Errorf("failed to parse validatorIndex %s | reason %v", validatorIndex, err))
	}
	if uintValidatorIndex != inputValidatorIndex {
		return false
	}
	if !bytes.Equal(fromDilithiumPubkey.Marshal(), credential.WithdrawalPK()) {
		return false
	}
	if !bytes.Equal(toExecutionAddress, credential.ZondWithdrawalAddress().Bytes()) ||
		!bytes.Equal(toExecutionAddress, misc.DecodeHex(inputExecutionAddress)) {
		return false
	}
	if !bytes.Equal(genesisValidatorsRoot, chainSetting.GenesisValidatorsRoot) {
		return false
	}

	message := &ethpbv2.DilithiumToExecutionChange{
		ValidatorIndex:      primitives.ValidatorIndex(uintValidatorIndex),
		FromDilithiumPubkey: fromDilithiumPubkey.Marshal(),
		ToExecutionAddress:  toExecutionAddress}
	root, err := message.HashTreeRoot()
	if err != nil {
		panic(fmt.Errorf("failed to generate hash tree root for message %v", err))
	}

	domain, err := signing.ComputeDomain(
		params.BeaconConfig().DomainDilithiumToExecutionChange,
		chainSetting.GenesisForkVersion,    /*forkVersion*/
		chainSetting.GenesisValidatorsRoot, /*genesisValidatorsRoot*/
	)

	signingData := &ethpb.SigningData{
		ObjectRoot: root[:],
		Domain:     domain,
	}

	signingRoot, err := signingData.HashTreeRoot()
	if err != nil {
		panic(fmt.Errorf("failed to generate hash tree root for signingData %v", err))
	}
	sizedPK := misc.ToSizedDilithiumPublicKey(credential.WithdrawalPK())
	return dilithium2.Verify(signingRoot[:], misc.ToSizedDilithiumSignature(signature.Marshal()), &sizedPK)
}

func ValidateDilithiumWithdrawalCredentialsMatching(dilithiumWithdrawalCredential string, credential *Credential) bool {
	binDilithiumWithdrawalCredential := misc.DecodeHex(dilithiumWithdrawalCredential)
	sha256Hash := sha256.Sum256(credential.WithdrawalPK())
	return bytes.Equal(binDilithiumWithdrawalCredential[1:], sha256Hash[1:])
}
