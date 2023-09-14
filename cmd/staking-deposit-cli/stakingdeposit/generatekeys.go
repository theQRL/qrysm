package stakingdeposit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"github.com/cyyber/qrysm/v4/beacon-chain/core/signing"
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/config"
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/misc"
	"github.com/cyyber/qrysm/v4/config/params"
	"github.com/cyyber/qrysm/v4/crypto/dilithium"
	ethpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
)

func GenerateKeys(validatorStartIndex, numValidators uint64,
	seed, folder, chain, keystorePassword, executionAddress string) {
	chainSettings, ok := config.GetConfig().ChainSettings[chain]
	if !ok {
		panic(fmt.Errorf("cannot find chain settings for %s", chain))
	}
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		err := os.MkdirAll(folder, 0775)
		if err != nil {
			panic(fmt.Errorf("cannot create folder. reason: %v", err))
		}
	}

	amounts := make([]uint64, numValidators)
	for i := uint64(0); i < numValidators; i++ {
		amounts[i] = params.BeaconConfig().MaxEffectiveBalance
	}

	credentials, err := NewCredentialsFromSeed(seed, numValidators, amounts, chainSettings, validatorStartIndex, executionAddress)
	if err != nil {
		panic(fmt.Errorf("new credentials from mnemonic failed. reason: %v", err))
	}
	keystoreFileFolders, err := credentials.ExportKeystores(keystorePassword, folder)
	if err != nil {
		panic(fmt.Errorf("export keystores failed. reason: %v", err))
	}
	depositFile, err := credentials.ExportDepositDataJSON(folder)
	if !credentials.VerifyKeystores(keystoreFileFolders, keystorePassword) {
		panic("failed to verify the keystores")
	}
	if !VerifyDepositDataJSON(depositFile, credentials.credentials) {
		panic("failed to verify the deposit data JSON files")
	}

	fmt.Println("Please note down your Dilithium seed: ", seed)
}

func VerifyDepositDataJSON(fileFolder string, credentials []*Credential) bool {
	data, err := os.ReadFile(fileFolder)
	if err != nil {
		panic(fmt.Errorf("failed to read file %s | reason %v", fileFolder, err))
	}

	var depositDataList []*DepositData
	if err := json.Unmarshal(data, &depositDataList); err != nil {
		panic(fmt.Errorf("failed to unmarshal data to []*DepositData from file %s | reason %v ",
			fileFolder, err))
	}
	for i, credential := range credentials {
		if !validateDeposit(depositDataList[i], credential) {
			return false
		}
	}
	return true
}

func validateDeposit(depositData *DepositData, credential *Credential) bool {
	signingSeed := misc.StrSeedToBinSeed(credential.signingSeed)
	depositKey, err := dilithium.SecretKeyFromBytes(signingSeed[:])
	if err != nil {
		panic(fmt.Errorf("failed to derive dilithium depositKey from signingSeed | reason %v", err))
	}
	pubKey, err := hex.DecodeString(depositData.PubKey)
	if err != nil {
		panic(fmt.Errorf("failed to decode pubkey"))
	}

	withdrawalCredentials, err := hex.DecodeString(depositData.WithdrawalCredentials)
	if err != nil {
		panic(fmt.Errorf("failed to decode withdrawalCredentials"))
	}

	signature, err := hex.DecodeString(depositData.Signature)
	if err != nil {
		panic(fmt.Errorf("failed to decode signature"))
	}

	if len(pubKey) != dilithium2.CryptoPublicKeyBytes {
		return false
	}
	if !reflect.DeepEqual(pubKey, depositKey.PublicKey().Marshal()) {
		return false
	}

	if len(withdrawalCredentials) != 32 {
		panic(fmt.Errorf("failed to derive dilithium depositKey from signingSeed | reason %v", err))
	}

	zeroBytes11 := make([]uint8, 11)
	if reflect.DeepEqual(withdrawalCredentials[0], params.BeaconConfig().ZondAddressWithdrawalPrefixByte) {
		if !reflect.DeepEqual(withdrawalCredentials[1:12], zeroBytes11) {
			panic("withdrawal credentials zero bytes not found for index 1:12")
		}
		if !reflect.DeepEqual(withdrawalCredentials[12:], credential.ZondWithdrawalAddress().Bytes()) {
			panic(fmt.Errorf("withdrawalCredentials[12:] %x mismatch with credential.ZondWithdrawalAddress %x",
				withdrawalCredentials[12:], credential.ZondWithdrawalAddress().Bytes()))
		}
	} else if reflect.DeepEqual(withdrawalCredentials[0], params.BeaconConfig().DilithiumWithdrawalPrefixByte) {
		hashWithdrawalPK := sha256.Sum256(credential.WithdrawalPK())
		if !reflect.DeepEqual(withdrawalCredentials[1:], hashWithdrawalPK[1:]) {
			panic(fmt.Errorf("withdrawalCredentials[1:] %x mismatch with hashWithdrawalPK[1:] %x",
				withdrawalCredentials[1:], hashWithdrawalPK[1:]))
		}
	} else {
		panic(fmt.Errorf("invalid prefixbyte withdrawalCredentials[0] %x", withdrawalCredentials[0]))
	}

	if len(signature) != dilithium2.CryptoBytes {
		panic(fmt.Errorf("invalid dilitihium signature length %d", len(signature)))
	}

	if depositData.Amount > params.BeaconConfig().MaxEffectiveBalance {
		return false
	}

	depositMessage := &ethpb.DepositMessage{
		PublicKey:             depositKey.PublicKey().Marshal(),
		WithdrawalCredentials: withdrawalCredentials,
		Amount:                depositData.Amount,
	}
	root, err := depositMessage.HashTreeRoot()
	if err != nil {
		panic(fmt.Errorf("could not get depositMessage.HashTreeRoot() | reason %v", err))
	}
	domain, err := signing.ComputeDomain(
		params.BeaconConfig().DomainDeposit,
		config.ToHex(depositData.ForkVersion), /*forkVersion*/
		nil,                                   /*genesisValidatorsRoot*/
	)
	signingData := &ethpb.SigningData{
		ObjectRoot: root[:],
		Domain:     domain,
	}
	ctrRoot, err := signingData.HashTreeRoot()
	if err != nil {
		panic(fmt.Errorf("could not get signingData.HashTreeRoot() | reason %v", err))
	}
	sig, err := dilithium.SignatureFromBytes(signature)
	publicKey, err := dilithium.PublicKeyFromBytes(pubKey)

	if !sig.Verify(publicKey, ctrRoot[:]) {
		return false
	}

	return true
}
