package stakingdeposit

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"github.com/theQRL/go-qrl/common"
	goqrllib_misc "github.com/theQRL/go-qrllib/wallet/misc"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/config"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/misc"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/crypto/randao"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

func GenerateKeys(validatorStartIndex, numValidators uint64,
	seed, folder, chain, keystorePassword string, withdrawalAddr common.Address, lightKDF bool) {
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
	for i := range numValidators {
		amounts[i] = params.BeaconConfig().MaxEffectiveBalance
	}

	fmt.Println("Generation keystores, this may take a while...")
	credentials, err := NewCredentialsFromSeed(seed, numValidators, amounts, chainSettings, validatorStartIndex, withdrawalAddr)
	if err != nil {
		panic(fmt.Errorf("new credentials from mnemonic failed. reason: %v", err))
	}
	keystoreFileFolders, err := credentials.ExportKeystores(keystorePassword, folder, lightKDF)
	if err != nil {
		panic(fmt.Errorf("export keystores failed. reason: %v", err))
	}
	depositFile, err := credentials.ExportDepositDataJSON(folder)
	if err != nil {
		panic(fmt.Errorf("failed to export deposit data. reason: %v", err))
	}
	if !credentials.VerifyKeystores(keystoreFileFolders, keystorePassword) {
		panic("failed to verify the keystores")
	}
	if !VerifyDepositDataJSON(depositFile, credentials.credentials) {
		panic("failed to verify the deposit data JSON files")
	}

	// TODO (cyyber): Replace this with extended seed
	fmt.Println("Please note down your ML-DSA-87 seed: ", seed)
	binSeed := misc.StrSeedToBinSeed(seed)
	mnemonic, err := goqrllib_misc.BinToMnemonic(binSeed[:])
	if err != nil {
		panic(fmt.Errorf("failed to convert seed %v to mnemonic | reason %v", seed, err))
	}
	fmt.Println("Mnemonic: ", mnemonic)
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
	depositKey, err := ml_dsa_87.SecretKeyFromSeed(signingSeed[:])
	if err != nil {
		panic(fmt.Errorf("failed to derive ML-DSA-87 depositKey from signingSeed | reason %v", err))
	}
	pubKey := misc.DecodeHex(depositData.PubKey)

	withdrawalRecipient := misc.DecodeHex(depositData.WithdrawalRecipient)

	signature := misc.DecodeHex(depositData.Signature)

	if len(pubKey) != field_params.MLDSA87PubkeyLength {
		return false
	}
	if !reflect.DeepEqual(pubKey, depositKey.PublicKey().Marshal()) {
		return false
	}

	if len(withdrawalRecipient) != field_params.WithdrawalRecipientLength {
		panic(fmt.Errorf("invalid withdrawal recipient length %d, want %d",
			len(withdrawalRecipient), field_params.WithdrawalRecipientLength))
	}

	if !reflect.DeepEqual(withdrawalRecipient, credential.withdrawalAddress.Bytes()) {
		panic(fmt.Errorf("withdrawalRecipient %x mismatch with credential.QRLWithdrawalAddress %x",
			withdrawalRecipient, credential.withdrawalAddress.Bytes()))
	}

	if len(signature) != field_params.MLDSA87SignatureLength {
		panic(fmt.Errorf("invalid dilitihium signature length %d", len(signature)))
	}

	// The deposit contract reverts below MIN_DEPOSIT_AMOUNT, so refuse here and
	// say why instead of letting the user send a transaction that fails.
	if depositData.Amount < params.BeaconConfig().MinDepositAmount || depositData.Amount > params.BeaconConfig().MaxEffectiveBalance {
		fmt.Printf("deposit amount %d shor for pubkey %s is outside the allowed range [%d, %d] shor\n",
			depositData.Amount, depositData.PubKey, params.BeaconConfig().MinDepositAmount, params.BeaconConfig().MaxEffectiveBalance)
		return false
	}

	randaoCommitment := misc.DecodeHex(depositData.RandaoCommitment)
	if len(randaoCommitment) != field_params.RandaoCommitmentLength {
		panic(fmt.Errorf("invalid randao commitment length %d, want %d",
			len(randaoCommitment), field_params.RandaoCommitmentLength))
	}
	if expected := randao.Commitment(signingSeed[:], randao.DefaultLayers); !reflect.DeepEqual(randaoCommitment, expected[:]) {
		return false
	}

	depositMessage := &qrysmpb.DepositMessage{
		PublicKey:           depositKey.PublicKey().Marshal(),
		WithdrawalRecipient: withdrawalRecipient,
		Amount:              depositData.Amount,
		RandaoCommitment:    randaoCommitment,
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
	if err != nil {
		panic(fmt.Errorf("failed to compute domain | reason %v", err))
	}
	signingData := &qrysmpb.SigningData{
		ObjectRoot: root[:],
		Domain:     domain,
	}
	ctrRoot, err := signingData.HashTreeRoot()
	if err != nil {
		panic(fmt.Errorf("could not get signingData.HashTreeRoot() | reason %v", err))
	}
	sig, err := ml_dsa_87.SignatureFromBytes(signature)
	if err != nil {
		panic(fmt.Errorf("could not parse signature bytes | reason %v", err))
	}
	publicKey, err := ml_dsa_87.PublicKeyFromBytes(pubKey)
	if err != nil {
		panic(fmt.Errorf("could not parse public key bytes | reason %v", err))
	}

	if !sig.Verify(publicKey, ctrRoot[:]) {
		return false
	}

	return true
}
