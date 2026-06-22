package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/cmd/validator/flags"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	validatorserviceconfig "github.com/theQRL/qrysm/config/validator/service"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/consensus-types/validator"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	keystorev1 "github.com/theQRL/qrysm/pkg/go-qrl-wallet-encryptor-keystore"
	qrlpbservice "github.com/theQRL/qrysm/proto/qrl/service"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	validatormock "github.com/theQRL/qrysm/testing/validator-mock"
	"github.com/theQRL/qrysm/validator/accounts"
	"github.com/theQRL/qrysm/validator/accounts/iface"
	mock "github.com/theQRL/qrysm/validator/accounts/testing"
	"github.com/theQRL/qrysm/validator/client"
	"github.com/theQRL/qrysm/validator/db/kv"
	dbtest "github.com/theQRL/qrysm/validator/db/testing"
	"github.com/theQRL/qrysm/validator/keymanager"
	"github.com/theQRL/qrysm/validator/keymanager/local"
	"github.com/theQRL/qrysm/validator/slashing-protection-history/format"
	mocks "github.com/theQRL/qrysm/validator/testing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const strongPass = "29384283xasjasd32%%&*@*#*"

var defaultWalletPath = filepath.Join(flags.DefaultValidatorDir(), flags.WalletDefaultDirName)

func setupWalletDir(t testing.TB) string {
	walletDir := filepath.Join(t.TempDir(), "wallet")
	require.NoError(t, os.MkdirAll(walletDir, os.ModePerm))
	return walletDir
}

func TestServer_ListKeystores(t *testing.T) {
	t.Run("wallet not ready", func(t *testing.T) {
		s := Server{}
		_, err := s.ListKeystores(context.Background(), &emptypb.Empty{})
		require.ErrorContains(t, "Qrysm Wallet not initialized. Please create a new wallet.", err)
	})
	ctx := context.Background()
	localWalletDir := setupWalletDir(t)
	defaultWalletPath = localWalletDir
	opts := []accounts.Option{
		accounts.WithWalletDir(defaultWalletPath),
		// accounts.WithKeymanagerType(keymanager.Derived),
		accounts.WithKeymanagerType(keymanager.Local),
		accounts.WithWalletPassword(strongPass),
		// accounts.WithSkipMnemonicConfirm(true),
	}
	acc, err := accounts.NewCLIManager(opts...)
	require.NoError(t, err)
	w, err := acc.WalletCreate(ctx)
	require.NoError(t, err)
	km, err := w.InitializeKeymanager(ctx, iface.InitKeymanagerConfig{ListenForChanges: false})
	require.NoError(t, err)
	vs, err := client.NewValidatorService(ctx, &client.Config{
		Wallet: w,
		Validator: &mock.MockValidator{
			Km: km,
		},
	})
	require.NoError(t, err)
	s := &Server{
		walletInitialized: true,
		wallet:            w,
		validatorService:  vs,
	}
	numAccounts := 50
	// dr, ok := km.(*derived.Keymanager)
	password := "test"
	dr, ok := km.(*local.Keymanager)
	require.Equal(t, true, ok)
	// err = dr.RecoverAccountsFromMnemonic(ctx, mocks.TestMnemonic, derived.DefaultMnemonicLanguage, "", numAccounts)
	keystores := make([]*keymanager.Keystore, numAccounts)
	passwords := make([]string, numAccounts)
	for i := range numAccounts {
		keystores[i] = createRandomKeystore(t, password)
		passwords[i] = password
	}
	_, err = dr.ImportKeystores(ctx, keystores, passwords)
	require.NoError(t, err)
	expectedKeys, err := dr.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)

	t.Run("returns proper data with existing keystores", func(t *testing.T) {
		resp, err := s.ListKeystores(context.Background(), &emptypb.Empty{})
		require.NoError(t, err)
		require.Equal(t, numAccounts, len(resp.Data))
		for i := range numAccounts {
			require.DeepEqual(t, expectedKeys[i][:], resp.Data[i].ValidatingPubkey)
			// require.Equal(
			// 	t,
			// 	fmt.Sprintf(derived.ValidatingKeyDerivationPathTemplate, i),
			// 	resp.Data[i].DerivationPath,
			// )
		}
	})
}

func TestServer_ImportKeystores(t *testing.T) {
	t.Run("wallet not ready", func(t *testing.T) {
		s := Server{}
		response, err := s.ImportKeystores(context.Background(), &qrlpbservice.ImportKeystoresRequest{})
		require.NoError(t, err)
		require.Equal(t, 0, len(response.Data))
	})
	ctx := context.Background()
	localWalletDir := setupWalletDir(t)
	defaultWalletPath = localWalletDir
	opts := []accounts.Option{
		accounts.WithWalletDir(defaultWalletPath),
		// accounts.WithKeymanagerType(keymanager.Derived),
		accounts.WithKeymanagerType(keymanager.Local),
		accounts.WithWalletPassword(strongPass),
		// accounts.WithSkipMnemonicConfirm(true),
	}
	acc, err := accounts.NewCLIManager(opts...)
	require.NoError(t, err)
	w, err := acc.WalletCreate(ctx)
	require.NoError(t, err)
	km, err := w.InitializeKeymanager(ctx, iface.InitKeymanagerConfig{ListenForChanges: false})
	require.NoError(t, err)
	vs, err := client.NewValidatorService(ctx, &client.Config{
		Wallet: w,
		Validator: &mock.MockValidator{
			Km: km,
		},
	})
	require.NoError(t, err)
	s := &Server{
		walletInitialized: true,
		wallet:            w,
		validatorService:  vs,
	}
	t.Run("200 response even if faulty keystore in request", func(t *testing.T) {
		response, err := s.ImportKeystores(context.Background(), &qrlpbservice.ImportKeystoresRequest{
			Keystores: []string{"hi"},
			Passwords: []string{"hi"},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(response.Data))
		require.Equal(t, qrlpbservice.ImportedKeystoreStatus_ERROR, response.Data[0].Status)
	})
	t.Run("200 response even if  no passwords in request", func(t *testing.T) {
		response, err := s.ImportKeystores(context.Background(), &qrlpbservice.ImportKeystoresRequest{
			Keystores: []string{"hi"},
			Passwords: []string{},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(response.Data))
		require.Equal(t, qrlpbservice.ImportedKeystoreStatus_ERROR, response.Data[0].Status)
	})
	t.Run("200 response even if  keystores more than passwords in request", func(t *testing.T) {
		response, err := s.ImportKeystores(context.Background(), &qrlpbservice.ImportKeystoresRequest{
			Keystores: []string{"hi", "hi"},
			Passwords: []string{"hi"},
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(response.Data))
		require.Equal(t, qrlpbservice.ImportedKeystoreStatus_ERROR, response.Data[0].Status)
	})
	t.Run("200 response even if number of passwords does not match number of keystores", func(t *testing.T) {
		response, err := s.ImportKeystores(context.Background(), &qrlpbservice.ImportKeystoresRequest{
			Keystores: []string{"hi"},
			Passwords: []string{"hi", "hi"},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(response.Data))
		require.Equal(t, qrlpbservice.ImportedKeystoreStatus_ERROR, response.Data[0].Status)
	})
	t.Run("200 response even if faulty slashing protection data", func(t *testing.T) {
		numKeystores := 5
		password := "12345678"
		encodedKeystores := make([]string, numKeystores)
		passwords := make([]string, numKeystores)
		for i := range numKeystores {
			enc, err := json.Marshal(createRandomKeystore(t, password))
			encodedKeystores[i] = string(enc)
			require.NoError(t, err)
			passwords[i] = password
		}
		resp, err := s.ImportKeystores(context.Background(), &qrlpbservice.ImportKeystoresRequest{
			Keystores:          encodedKeystores,
			Passwords:          passwords,
			SlashingProtection: "foobar",
		})
		require.NoError(t, err)
		require.Equal(t, numKeystores, len(resp.Data))
		for _, st := range resp.Data {
			require.Equal(t, qrlpbservice.ImportedKeystoreStatus_ERROR, st.Status)
		}
	})
	t.Run("returns proper statuses for keystores in request", func(t *testing.T) {
		numKeystores := 5
		password := "12345678"
		keystores := make([]*keymanager.Keystore, numKeystores)
		passwords := make([]string, numKeystores)
		publicKeys := make([][field_params.MLDSA87PubkeyLength]byte, numKeystores)
		for i := range numKeystores {
			keystores[i] = createRandomKeystore(t, password)
			pubKey, err := hex.DecodeString(keystores[i].Pubkey)
			require.NoError(t, err)
			publicKeys[i] = bytesutil.ToBytes2592(pubKey)
			passwords[i] = password
		}

		// Create a validator database.
		validatorDB, err := kv.NewKVStore(ctx, defaultWalletPath, &kv.Config{
			PubKeys: publicKeys,
		})
		require.NoError(t, err)
		s.valDB = validatorDB

		// Have to close it after import is done otherwise it complains db is not open.
		defer func() {
			require.NoError(t, validatorDB.Close())
		}()
		encodedKeystores := make([]string, numKeystores)
		for i := range numKeystores {
			enc, err := json.Marshal(keystores[i])
			require.NoError(t, err)
			encodedKeystores[i] = string(enc)
		}

		// Generate mock slashing history.
		attestingHistory := make([][]*kv.AttestationRecord, 0)
		proposalHistory := make([]kv.ProposalHistoryForPubkey, len(publicKeys))
		for i := range publicKeys {
			proposalHistory[i].Proposals = make([]kv.Proposal, 0)
		}
		mockJSON, err := mocks.MockSlashingProtectionJSON(publicKeys, attestingHistory, proposalHistory)
		require.NoError(t, err)

		// JSON encode the protection JSON and save it.
		encodedSlashingProtection, err := json.Marshal(mockJSON)
		require.NoError(t, err)

		resp, err := s.ImportKeystores(context.Background(), &qrlpbservice.ImportKeystoresRequest{
			Keystores:          encodedKeystores,
			Passwords:          passwords,
			SlashingProtection: string(encodedSlashingProtection),
		})
		require.NoError(t, err)
		require.Equal(t, numKeystores, len(resp.Data))
		for _, status := range resp.Data {
			require.Equal(t, qrlpbservice.ImportedKeystoreStatus_IMPORTED, status.Status)
		}
	})
}

// TODO(now.youtrack.cloud/issue/TQ-2)
/*
func TestServer_ImportKeystores_WrongKeymanagerKind(t *testing.T) {
	ctx := context.Background()
	w := wallet.NewWalletForWeb3Signer()
	root := make([]byte, fieldparams.RootLength)
	root[0] = 1
	km, err := w.InitializeKeymanager(ctx, iface.InitKeymanagerConfig{ListenForChanges: false, Web3SignerConfig: &remoteweb3signer.SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		PublicKeysURL:         "http://example.com/public_keys",
	}})
	require.NoError(t, err)
	vs, err := client.NewValidatorService(ctx, &client.Config{
		Wallet: w,
		Validator: &mock.MockValidator{
			Km: km,
		},
	})
	require.NoError(t, err)
	s := &Server{
		walletInitialized: true,
		wallet:            w,
		validatorService:  vs,
	}
	response, err := s.ImportKeystores(context.Background(), &qrlpbservice.ImportKeystoresRequest{
		Keystores: []string{"hi"},
		Passwords: []string{"hi"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(response.Data))
	require.Equal(t, qrlpbservice.ImportedKeystoreStatus_ERROR, response.Data[0].Status)
	require.Equal(t, "Keymanager kind cannot import keys", response.Data[0].Message)
}
*/

func TestServer_DeleteKeystores(t *testing.T) {
	t.Run("wallet not ready", func(t *testing.T) {
		s := Server{}
		response, err := s.DeleteKeystores(context.Background(), &qrlpbservice.DeleteKeystoresRequest{})
		require.NoError(t, err)
		require.Equal(t, 0, len(response.Data))
	})
	ctx := context.Background()
	srv := setupServerWithWallet(t)

	// We recover 3 accounts from a test mnemonic.
	numAccounts := 3
	km, er := srv.validatorService.Keymanager()
	require.NoError(t, er)
	// dr, ok := km.(*derived.Keymanager)
	dr, ok := km.(*local.Keymanager)
	require.Equal(t, true, ok)
	// err := dr.RecoverAccountsFromMnemonic(ctx, mocks.TestMnemonic, derived.DefaultMnemonicLanguage, "", numAccounts)
	password := "test"
	keystores := make([]*keymanager.Keystore, numAccounts)
	passwords := make([]string, numAccounts)
	for i := range numAccounts {
		keystores[i] = createRandomKeystore(t, password)
		passwords[i] = password
	}
	_, err := dr.ImportKeystores(ctx, keystores, passwords)
	require.NoError(t, err)
	publicKeys, err := dr.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)

	// Create a validator database.
	validatorDB, err := kv.NewKVStore(ctx, defaultWalletPath, &kv.Config{
		PubKeys: publicKeys,
	})
	require.NoError(t, err)
	srv.valDB = validatorDB

	// Have to close it after import is done otherwise it complains db is not open.
	defer func() {
		require.NoError(t, validatorDB.Close())
	}()

	// Generate mock slashing history.
	attestingHistory := make([][]*kv.AttestationRecord, 0)
	proposalHistory := make([]kv.ProposalHistoryForPubkey, len(publicKeys))
	for i := range publicKeys {
		proposalHistory[i].Proposals = make([]kv.Proposal, 0)
	}
	mockJSON, err := mocks.MockSlashingProtectionJSON(publicKeys, attestingHistory, proposalHistory)
	require.NoError(t, err)

	// JSON encode the protection JSON and save it.
	encoded, err := json.Marshal(mockJSON)
	require.NoError(t, err)

	_, err = srv.ImportSlashingProtection(ctx, &validatorpb.ImportSlashingProtectionRequest{
		SlashingProtectionJson: string(encoded),
	})
	require.NoError(t, err)

	t.Run("no slashing protection response if no keys in request even if we have a history in DB", func(t *testing.T) {
		resp, err := srv.DeleteKeystores(context.Background(), &qrlpbservice.DeleteKeystoresRequest{
			Pubkeys: nil,
		})
		require.NoError(t, err)
		require.Equal(t, "", resp.SlashingProtection)
	})

	// For ease of test setup, we'll give each public key a string identifier.
	publicKeysWithId := map[string][field_params.MLDSA87PubkeyLength]byte{
		"a": publicKeys[0],
		"b": publicKeys[1],
		"c": publicKeys[2],
	}

	type keyCase struct {
		id                 string
		wantProtectionData bool
	}
	tests := []struct {
		keys         []*keyCase
		wantStatuses []qrlpbservice.DeletedKeystoreStatus_Status
	}{
		{
			keys: []*keyCase{
				{id: "a", wantProtectionData: true},
				{id: "a", wantProtectionData: true},
				{id: "d"},
				{id: "c", wantProtectionData: true},
			},
			wantStatuses: []qrlpbservice.DeletedKeystoreStatus_Status{
				qrlpbservice.DeletedKeystoreStatus_DELETED,
				qrlpbservice.DeletedKeystoreStatus_NOT_ACTIVE,
				qrlpbservice.DeletedKeystoreStatus_NOT_FOUND,
				qrlpbservice.DeletedKeystoreStatus_DELETED,
			},
		},
		{
			keys: []*keyCase{
				{id: "a", wantProtectionData: true},
				{id: "c", wantProtectionData: true},
			},
			wantStatuses: []qrlpbservice.DeletedKeystoreStatus_Status{
				qrlpbservice.DeletedKeystoreStatus_NOT_ACTIVE,
				qrlpbservice.DeletedKeystoreStatus_NOT_ACTIVE,
			},
		},
		{
			keys: []*keyCase{
				{id: "x"},
			},
			wantStatuses: []qrlpbservice.DeletedKeystoreStatus_Status{
				qrlpbservice.DeletedKeystoreStatus_NOT_FOUND,
			},
		},
	}
	for _, tc := range tests {
		keys := make([][]byte, len(tc.keys))
		for i := 0; i < len(tc.keys); i++ {
			pk := publicKeysWithId[tc.keys[i].id]
			keys[i] = pk[:]
		}
		resp, err := srv.DeleteKeystores(ctx, &qrlpbservice.DeleteKeystoresRequest{Pubkeys: keys})
		require.NoError(t, err)
		require.Equal(t, len(keys), len(resp.Data))
		slashingProtectionData := &format.EIPSlashingProtectionFormat{}
		require.NoError(t, json.Unmarshal([]byte(resp.SlashingProtection), slashingProtectionData))
		require.Equal(t, true, len(slashingProtectionData.Data) > 0)

		for i := 0; i < len(tc.keys); i++ {
			require.Equal(
				t,
				tc.wantStatuses[i],
				resp.Data[i].Status,
				fmt.Sprintf("Checking status for key %s", tc.keys[i].id),
			)
			if tc.keys[i].wantProtectionData {
				// We check that we can find the key in the slashing protection data.
				var found bool
				for _, dt := range slashingProtectionData.Data {
					if dt.Pubkey == fmt.Sprintf("%#x", keys[i]) {
						found = true
						break
					}
				}
				require.Equal(t, true, found)
			}
		}
	}
}

func TestServer_DeleteKeystores_FailedSlashingProtectionExport(t *testing.T) {
	ctx := context.Background()
	srv := setupServerWithWallet(t)

	// We recover 3 accounts from a test mnemonic.
	numAccounts := 3
	km, er := srv.validatorService.Keymanager()
	require.NoError(t, er)
	// dr, ok := km.(*derived.Keymanager)
	dr, ok := km.(*local.Keymanager)
	require.Equal(t, true, ok)
	// err := dr.RecoverAccountsFromMnemonic(ctx, mocks.TestMnemonic, derived.DefaultMnemonicLanguage, "", numAccounts)
	// require.NoError(t, err)
	password := "test"
	keystores := make([]*keymanager.Keystore, numAccounts)
	passwords := make([]string, numAccounts)
	for i := range numAccounts {
		keystores[i] = createRandomKeystore(t, password)
		passwords[i] = password
	}
	publicKeys, err := dr.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)

	// Create a validator database.
	validatorDB, err := kv.NewKVStore(ctx, defaultWalletPath, &kv.Config{
		PubKeys: publicKeys,
	})
	require.NoError(t, err)
	err = validatorDB.SaveGenesisValidatorsRoot(ctx, make([]byte, fieldparams.RootLength))
	require.NoError(t, err)
	srv.valDB = validatorDB

	// Have to close it after import is done otherwise it complains db is not open.
	defer func() {
		require.NoError(t, validatorDB.Close())
	}()

	response, err := srv.DeleteKeystores(context.Background(), &qrlpbservice.DeleteKeystoresRequest{
		Pubkeys: [][]byte{[]byte("a")},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(response.Data))
	require.Equal(t, qrlpbservice.DeletedKeystoreStatus_ERROR, response.Data[0].Status)
	require.Equal(t, "Non duplicate keys that were existing were deleted, but could not export slashing protection history.",
		response.Data[0].Message,
	)
}

// TODO(now.youtrack.cloud/issue/TQ-2)
/*
func TestServer_DeleteKeystores_WrongKeymanagerKind(t *testing.T) {
	ctx := context.Background()
	w := wallet.NewWalletForWeb3Signer()
	root := make([]byte, fieldparams.RootLength)
	root[0] = 1
	km, err := w.InitializeKeymanager(ctx, iface.InitKeymanagerConfig{ListenForChanges: false,
		Web3SignerConfig: &remoteweb3signer.SetupConfig{
			BaseEndpoint:          "http://example.com",
			GenesisValidatorsRoot: root,
			PublicKeysURL:         "http://example.com/public_keys",
		}})
	require.NoError(t, err)
	vs, err := client.NewValidatorService(ctx, &client.Config{
		Wallet: w,
		Validator: &mock.MockValidator{
			Km: km,
		},
	})
	require.NoError(t, err)
	s := &Server{
		walletInitialized: true,
		wallet:            w,
		validatorService:  vs,
	}
	_, err = s.DeleteKeystores(ctx, &qrlpbservice.DeleteKeystoresRequest{Pubkeys: [][]byte{[]byte("a")}})
	require.ErrorContains(t, "Wrong wallet type", err)
	require.ErrorContains(t, "Only Imported or Derived wallets can delete accounts", err)
}
*/

func setupServerWithWallet(t testing.TB) *Server {
	ctx := context.Background()
	localWalletDir := setupWalletDir(t)
	defaultWalletPath = localWalletDir
	opts := []accounts.Option{
		accounts.WithWalletDir(defaultWalletPath),
		// accounts.WithKeymanagerType(keymanager.Derived),
		accounts.WithKeymanagerType(keymanager.Local),
		accounts.WithWalletPassword(strongPass),
		// accounts.WithSkipMnemonicConfirm(true),
	}
	acc, err := accounts.NewCLIManager(opts...)
	require.NoError(t, err)
	w, err := acc.WalletCreate(ctx)
	require.NoError(t, err)
	km, err := w.InitializeKeymanager(ctx, iface.InitKeymanagerConfig{ListenForChanges: false})
	require.NoError(t, err)
	vs, err := client.NewValidatorService(ctx, &client.Config{
		Wallet: w,
		Validator: &mock.MockValidator{
			Km: km,
		},
	})
	require.NoError(t, err)

	return &Server{
		walletInitialized: true,
		wallet:            w,
		validatorService:  vs,
	}
}

func createRandomKeystore(t testing.TB, password string) *keymanager.Keystore {
	encryptor := keystorev1.New()
	id, err := uuid.NewRandom()
	require.NoError(t, err)
	validatingKey, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	pubKey := validatingKey.PublicKey().Marshal()
	cryptoFields, err := encryptor.Encrypt(validatingKey.Marshal(), password)
	require.NoError(t, err)
	return &keymanager.Keystore{
		Crypto:      cryptoFields,
		Pubkey:      fmt.Sprintf("%x", pubKey),
		ID:          id.String(),
		Version:     encryptor.Version(),
		Description: encryptor.Name(),
	}
}

/*
func TestServer_ListRemoteKeys(t *testing.T) {
	t.Run("wallet not ready", func(t *testing.T) {
		s := Server{}
		_, err := s.ListKeystores(context.Background(), &empty.Empty{})
		require.ErrorContains(t, "Qrysm Wallet not initialized. Please create a new wallet.", err)
	})
	ctx := context.Background()
	w := wallet.NewWalletForWeb3Signer()
	root := make([]byte, fieldparams.RootLength)
	root[0] = 1
	bytevalue, err := hexutil.Decode("0xe0a586bb51db522c31abcbce14e6cbf6a5bbc7b3331cdb76378ca1b98acff048c11099c2713f229c349c430a6aa5623fab8d39ec266e0e7d81543fc2e4b905ec7fba75b9ab3aae53e18e2a018297ebe4bb2d0a22bd13b60b938461d922ec81dfe152224c51abcccd4105799ee2b70b53cf2401a3c01664c20ab368c4c3ccc764be5063488750f79480adcac444e274fb46500aeb2449d2a81e44c3528c70554a9ecd5b25b39550d469a43f5ec2afce668aa6598aa1c5618e569bdf08ec700a21950d6d2df3337ff196b6fcb53de94e7e127dbd7edf9c5df70c41c715b48cf4e5ab5d0e1bc4d9ead578150f98244ea47dba29af25b12a72054618d0341ebbae8e5c61cf6583c0151fdcf1323ee3cd65f8f739dc621f2aa77f8dfe36a7cef15162972c25a193bd306918deb8d6395367586ee6a534340c07caf6496dc393a0189cf81325499132a012a2b8a6152be3d3d010aadba896af83d9d447741a66100f72da46c9282a8a9af5bfec0d84d88882ce0a090147dbcef2f100f8744094a8e3712c26d875996f56a92fd99a39537197f0bbe58bb706061426e62406a300626f64b7dd813c756c159cea82a6cf82b9890be40284720b9aa9c6f1a3a78bfb8607b438ec3665225fe21370770cfdbbc20ef6525362d413ab23e85d5f6ef38a43b44874828137ab977dd9a145913ecffe2700a225042b766158d26288434511014928efdb857df4217430e18bd6c370c8327b4451611c66a118193f1155ffeef32d9b26b02d04cd083964f53b59b5ffd02789be6e8aeae4f615afb39e53f5cfbf3d9ec23640fea711fc6751abe9b3606959ac12aeb827a76a515f27d0e0f1e003e00a91a1d20b97ecde53202d6f9d61a1d4b0bd7d4f0622c2a90d67ba40f59a450191aa340bcc7b3b3107830ce1fb791a97930fd68c6b9ea0848c0591edb0a6302e0984d7f096ccb980803bbeaae3550d8996a001ecba956d3c2bb20eaa33094071e639983693f64809e449c29b59bc0b4f1530ec273f366db337bc64d95a9e26cc21ed0685cb2c606b994505bdd6237dfdb414df7eee7544f34cdf5f0e6ca1e5280b493446cb883413e26e06a00354bced7a5fd410fd92ffc39443d9e8f208aec8d81d958c060203bbe75db0cb2b982524b5e91135d4ef671ebe6c55c24bdb00d89b78c7d8fed674d1fac6d6d61bb671a996d3efe27a254e40967cb60c3c7ac5814ca5e5768f268c7002ba200da9200fc5498d07833c4b25a111d35f64cd26b108a897616d4324984e0833937344b904b964d5f292eeba6075987b5cc092bd40697ef9b2ea95cbc1eed5bf3f6337145351e98291853c3bf75eb1a533817cab5dc8d87abf034696e8dc9ad20089d79086b8608cb07101b62ae744beba3cc71e75701d46c35f317ecd1f3bb6d6078bd8cf25a55bbd200d7bfb5c9e3e2167a6abe8a6636ca82bdd63c3007b39d9a57b9f258ee4bb94325b20744087ba3a2bbda513ed067b003a0d6a4197bae5776cb25899911f92e3cdd779931e98cb11846dac49480af3c2fc3596825ccbb7a7dfa3e714d8fc809acc57577dd448e477bff03a907f8410f2ae12a9ab4a3d7738315c07f42e5af416560aafa035ae4d4b72d5e59a45dcd4c91000cd8cef454c7a157276cb2610d2d08a7bc90550c85e317fdb2d83ee26f49198dd035bbe39d6eedfbc91a1cffb5682f0410204c281f3cb8d702258c77214d77e92f1e5f4db2a6be18911c5f3950a3228d1850722f4ce0a5d56a8acf2e0311290e1334a2bfbf1251d6cb46f2a028aaa7be144f38fdb222e8a2d6320f98796731847b2449774c025a452c72dfbb9f05959c88ed86256f5fcea5458c3e22340d8ad3c3ff548f03346c55f74d6ab3aff1311a302bb8c5cd55b44528fd08ecab030c1e47385ed27de5819fd798ada8858462de7fd55aa7239e03079d976669e52ff22bf9ab6fa4860064dd5033ca6ae1fec5c628e5bc2b190ae5483514d841a25b04d127d9c536e32f3bada7b46cbb14b5718c88ed826a8c19d1fd43a7f7ff6860a88adf9fcf1415eb2c56e12a7dc6a73e24bd7cbcc7fa39ce7358f20736adc11842e72a5107bcbe78f56bb56fe403d51ea531d7d4f2681fd05f5326d7e5dd7e889a3380f9dfe8124d8f258d6f9ba6f0f6467e787d996da6310196a70f551e64d1a9dc51fe907227f43a1fb54a572db183edb726375a7a1096daeb8d7b069bf8886d282dabdb7e9101fadcdfb23c57be75a193cc3459401d14836d250b197e6ae0e4818b2bea75db388bf36f311eff18ac14b9f0fe1a354d8d397439fd202d61d545f430676eb16c6ecc4c3f583fa8767d65cdc4f3155af47629cf1b0b833a12391b02b1781f1c31cb6b05160241b1e02c5889db631ad2fa905d608b2831b45529dd7550d5ff91d4b7ae23533b1f6875b38be0f26f4479cb75579d8612ff2cfec981344598c76054584f6350d296c2436e2d43f184556d4e6208483e010ba8bcdf413d659fab3353bb8f53f085dfe24910f28b82ae382047383e81922f2d05b13d073b3fbb8042c9c1dd6a073109afffac32117a6b4162387949a9c2b21661eb321a340978b4c43dcb8ce264d6e30751c1e91551f4c2efec349bf0f083db63f3bbbbc83be7eb044b17a7fbfdeedbdd80e76580a5082d7534cea34620997ee593fc0c725a9cc41f192cdcb85d2021f2dafea48f14f63d01329845c0533210075ac3d1b674a5535d37c5c5acbd8fdee0ef9d3dc66b9fdec661f3ed53d1c70c825937716af2d44510d07876b3d52c063e7ddca41faee15d3b81940dd50d41ad5791b4b37f44cecc11db9ce58c7555491ee822e8ff1d8b0dac3eb409f8b827561ea6b7d88af82892a53ff2d239c76a8a1a717b101c7b9d7db85a84a508276b8d1ba972d31089cce5dba3ed722ead0849d336b1002f41f1b1b93d2a7e56e5c222d21327d872534aae80e8f7020c4fda6fd4765bd94df4aa38c51924b356412ae0ceff6adfdad9b9c793ee6aec73a902f658ffd6af25abf374368e38a8b9e91b34d2eeca566eb39ffa67978077870b21279afc7760f38639ad6fa152af670f25de919fcecc16755bafff466a0b8d9910bf84bc5917a33ed76fd62c47a9a2ca68055668a13f11616b7f95cda26c2b09bbe8c83609af99ec41470bda5b12524a849950caf6fb96d908dabca97187858c83a54cb2dee7fcabbc0fea8d3ca1b860d1d7b5eb1dc2a687330d2fb237f55d97fbab4694e4037355548f1c20122da77eb0b7b90205989bb9ef52f76f88770eabf56f5d9ccaf572b3eadb7c9810e93b675e7e9ea26f8d8749fcb23c63993d62406db2a53996dc053e698e70360492e2c467e1baf2d76a9dc74f23c3be3d27685c5fd07a30d2aab3f2cd7fde563e29a3434ee6a51f795a5e114a3d6259732362126da789d82ee54dae91c3e2c060a4f79943068cb6a3ee5692587a67816aa5a9c5ff3805173c72a5ad2b0ebd8588253bae50da117d938901f8ffbf725ced16a76f9f53d782ebe1f0d6f6dfdaa4fe8f93ec6246b66e561c740fc7eaf6c771659e90f545b9e89221fc9450543424f0a14ad7484253251f658e56cf1cb161b4cee63c6c5b96cf8c06e6aa524c8209205de7fdbf1e233135755ed6300ed4c096764fe4dd4855f421d272cd63150db47bc6f47bf624798")
	require.NoError(t, err)
	pubkeys := [][field_params.MLDSA87PubkeyLength]byte{bytesutil.ToBytes2592(bytevalue)}
	config := &remoteweb3signer.SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		ProvidedPublicKeys:    pubkeys,
	}
	km, err := w.InitializeKeymanager(ctx, iface.InitKeymanagerConfig{ListenForChanges: false, Web3SignerConfig: config})
	require.NoError(t, err)
	vs, err := client.NewValidatorService(ctx, &client.Config{
		Wallet: w,
		Validator: &mock.MockValidator{
			Km: km,
		},
		Web3SignerConfig: config,
	})
	require.NoError(t, err)
	s := &Server{
		walletInitialized: true,
		wallet:            w,
		validatorService:  vs,
	}
	expectedKeys, err := km.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)

	t.Run("returns proper data with existing pub keystores", func(t *testing.T) {
		resp, err := s.ListRemoteKeys(context.Background(), &empty.Empty{})
		require.NoError(t, err)
		for i := 0; i < len(resp.Data); i++ {
			require.DeepEqual(t, expectedKeys[i][:], resp.Data[i].Pubkey)
		}
	})
}


func TestServer_ImportRemoteKeys(t *testing.T) {
	t.Run("wallet not ready", func(t *testing.T) {
		s := Server{}
		_, err := s.ListKeystores(context.Background(), &empty.Empty{})
		require.ErrorContains(t, "Qrysm Wallet not initialized. Please create a new wallet.", err)
	})
	ctx := context.Background()
	w := wallet.NewWalletForWeb3Signer()
	root := make([]byte, fieldparams.RootLength)
	root[0] = 1
	config := &remoteweb3signer.SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		ProvidedPublicKeys:    nil,
	}
	km, err := w.InitializeKeymanager(ctx, iface.InitKeymanagerConfig{ListenForChanges: false, Web3SignerConfig: config})
	require.NoError(t, err)
	vs, err := client.NewValidatorService(ctx, &client.Config{
		Wallet: w,
		Validator: &mock.MockValidator{
			Km: km,
		},
		Web3SignerConfig: config,
	})
	require.NoError(t, err)
	s := &Server{
		walletInitialized: true,
		wallet:            w,
		validatorService:  vs,
	}
	bytevalue, err := hexutil.Decode("0xe0a586bb51db522c31abcbce14e6cbf6a5bbc7b3331cdb76378ca1b98acff048c11099c2713f229c349c430a6aa5623fab8d39ec266e0e7d81543fc2e4b905ec7fba75b9ab3aae53e18e2a018297ebe4bb2d0a22bd13b60b938461d922ec81dfe152224c51abcccd4105799ee2b70b53cf2401a3c01664c20ab368c4c3ccc764be5063488750f79480adcac444e274fb46500aeb2449d2a81e44c3528c70554a9ecd5b25b39550d469a43f5ec2afce668aa6598aa1c5618e569bdf08ec700a21950d6d2df3337ff196b6fcb53de94e7e127dbd7edf9c5df70c41c715b48cf4e5ab5d0e1bc4d9ead578150f98244ea47dba29af25b12a72054618d0341ebbae8e5c61cf6583c0151fdcf1323ee3cd65f8f739dc621f2aa77f8dfe36a7cef15162972c25a193bd306918deb8d6395367586ee6a534340c07caf6496dc393a0189cf81325499132a012a2b8a6152be3d3d010aadba896af83d9d447741a66100f72da46c9282a8a9af5bfec0d84d88882ce0a090147dbcef2f100f8744094a8e3712c26d875996f56a92fd99a39537197f0bbe58bb706061426e62406a300626f64b7dd813c756c159cea82a6cf82b9890be40284720b9aa9c6f1a3a78bfb8607b438ec3665225fe21370770cfdbbc20ef6525362d413ab23e85d5f6ef38a43b44874828137ab977dd9a145913ecffe2700a225042b766158d26288434511014928efdb857df4217430e18bd6c370c8327b4451611c66a118193f1155ffeef32d9b26b02d04cd083964f53b59b5ffd02789be6e8aeae4f615afb39e53f5cfbf3d9ec23640fea711fc6751abe9b3606959ac12aeb827a76a515f27d0e0f1e003e00a91a1d20b97ecde53202d6f9d61a1d4b0bd7d4f0622c2a90d67ba40f59a450191aa340bcc7b3b3107830ce1fb791a97930fd68c6b9ea0848c0591edb0a6302e0984d7f096ccb980803bbeaae3550d8996a001ecba956d3c2bb20eaa33094071e639983693f64809e449c29b59bc0b4f1530ec273f366db337bc64d95a9e26cc21ed0685cb2c606b994505bdd6237dfdb414df7eee7544f34cdf5f0e6ca1e5280b493446cb883413e26e06a00354bced7a5fd410fd92ffc39443d9e8f208aec8d81d958c060203bbe75db0cb2b982524b5e91135d4ef671ebe6c55c24bdb00d89b78c7d8fed674d1fac6d6d61bb671a996d3efe27a254e40967cb60c3c7ac5814ca5e5768f268c7002ba200da9200fc5498d07833c4b25a111d35f64cd26b108a897616d4324984e0833937344b904b964d5f292eeba6075987b5cc092bd40697ef9b2ea95cbc1eed5bf3f6337145351e98291853c3bf75eb1a533817cab5dc8d87abf034696e8dc9ad20089d79086b8608cb07101b62ae744beba3cc71e75701d46c35f317ecd1f3bb6d6078bd8cf25a55bbd200d7bfb5c9e3e2167a6abe8a6636ca82bdd63c3007b39d9a57b9f258ee4bb94325b20744087ba3a2bbda513ed067b003a0d6a4197bae5776cb25899911f92e3cdd779931e98cb11846dac49480af3c2fc3596825ccbb7a7dfa3e714d8fc809acc57577dd448e477bff03a907f8410f2ae12a9ab4a3d7738315c07f42e5af416560aafa035ae4d4b72d5e59a45dcd4c91000cd8cef454c7a157276cb2610d2d08a7bc90550c85e317fdb2d83ee26f49198dd035bbe39d6eedfbc91a1cffb5682f0410204c281f3cb8d702258c77214d77e92f1e5f4db2a6be18911c5f3950a3228d1850722f4ce0a5d56a8acf2e0311290e1334a2bfbf1251d6cb46f2a028aaa7be144f38fdb222e8a2d6320f98796731847b2449774c025a452c72dfbb9f05959c88ed86256f5fcea5458c3e22340d8ad3c3ff548f03346c55f74d6ab3aff1311a302bb8c5cd55b44528fd08ecab030c1e47385ed27de5819fd798ada8858462de7fd55aa7239e03079d976669e52ff22bf9ab6fa4860064dd5033ca6ae1fec5c628e5bc2b190ae5483514d841a25b04d127d9c536e32f3bada7b46cbb14b5718c88ed826a8c19d1fd43a7f7ff6860a88adf9fcf1415eb2c56e12a7dc6a73e24bd7cbcc7fa39ce7358f20736adc11842e72a5107bcbe78f56bb56fe403d51ea531d7d4f2681fd05f5326d7e5dd7e889a3380f9dfe8124d8f258d6f9ba6f0f6467e787d996da6310196a70f551e64d1a9dc51fe907227f43a1fb54a572db183edb726375a7a1096daeb8d7b069bf8886d282dabdb7e9101fadcdfb23c57be75a193cc3459401d14836d250b197e6ae0e4818b2bea75db388bf36f311eff18ac14b9f0fe1a354d8d397439fd202d61d545f430676eb16c6ecc4c3f583fa8767d65cdc4f3155af47629cf1b0b833a12391b02b1781f1c31cb6b05160241b1e02c5889db631ad2fa905d608b2831b45529dd7550d5ff91d4b7ae23533b1f6875b38be0f26f4479cb75579d8612ff2cfec981344598c76054584f6350d296c2436e2d43f184556d4e6208483e010ba8bcdf413d659fab3353bb8f53f085dfe24910f28b82ae382047383e81922f2d05b13d073b3fbb8042c9c1dd6a073109afffac32117a6b4162387949a9c2b21661eb321a340978b4c43dcb8ce264d6e30751c1e91551f4c2efec349bf0f083db63f3bbbbc83be7eb044b17a7fbfdeedbdd80e76580a5082d7534cea34620997ee593fc0c725a9cc41f192cdcb85d2021f2dafea48f14f63d01329845c0533210075ac3d1b674a5535d37c5c5acbd8fdee0ef9d3dc66b9fdec661f3ed53d1c70c825937716af2d44510d07876b3d52c063e7ddca41faee15d3b81940dd50d41ad5791b4b37f44cecc11db9ce58c7555491ee822e8ff1d8b0dac3eb409f8b827561ea6b7d88af82892a53ff2d239c76a8a1a717b101c7b9d7db85a84a508276b8d1ba972d31089cce5dba3ed722ead0849d336b1002f41f1b1b93d2a7e56e5c222d21327d872534aae80e8f7020c4fda6fd4765bd94df4aa38c51924b356412ae0ceff6adfdad9b9c793ee6aec73a902f658ffd6af25abf374368e38a8b9e91b34d2eeca566eb39ffa67978077870b21279afc7760f38639ad6fa152af670f25de919fcecc16755bafff466a0b8d9910bf84bc5917a33ed76fd62c47a9a2ca68055668a13f11616b7f95cda26c2b09bbe8c83609af99ec41470bda5b12524a849950caf6fb96d908dabca97187858c83a54cb2dee7fcabbc0fea8d3ca1b860d1d7b5eb1dc2a687330d2fb237f55d97fbab4694e4037355548f1c20122da77eb0b7b90205989bb9ef52f76f88770eabf56f5d9ccaf572b3eadb7c9810e93b675e7e9ea26f8d8749fcb23c63993d62406db2a53996dc053e698e70360492e2c467e1baf2d76a9dc74f23c3be3d27685c5fd07a30d2aab3f2cd7fde563e29a3434ee6a51f795a5e114a3d6259732362126da789d82ee54dae91c3e2c060a4f79943068cb6a3ee5692587a67816aa5a9c5ff3805173c72a5ad2b0ebd8588253bae50da117d938901f8ffbf725ced16a76f9f53d782ebe1f0d6f6dfdaa4fe8f93ec6246b66e561c740fc7eaf6c771659e90f545b9e89221fc9450543424f0a14ad7484253251f658e56cf1cb161b4cee63c6c5b96cf8c06e6aa524c8209205de7fdbf1e233135755ed6300ed4c096764fe4dd4855f421d272cd63150db47bc6f47bf624798")
	require.NoError(t, err)
	remoteKeys := []*qrlpbservice.ImportRemoteKeysRequest_Keystore{
		{
			Pubkey: bytevalue,
		},
	}

	t.Run("returns proper data with existing pub keystores", func(t *testing.T) {
		resp, err := s.ImportRemoteKeys(context.Background(), &qrlpbservice.ImportRemoteKeysRequest{
			RemoteKeys: remoteKeys,
		})
		expectedStatuses := []*qrlpbservice.ImportedRemoteKeysStatus{
			{
				Status:  qrlpbservice.ImportedRemoteKeysStatus_IMPORTED,
				Message: fmt.Sprintf("Successfully added pubkey: %v", hexutil.Encode(bytevalue)),
			},
		}
		require.NoError(t, err)
		for i := 0; i < len(resp.Data); i++ {
			require.DeepEqual(t, expectedStatuses[i], resp.Data[i])
		}
	})
}

func TestServer_DeleteRemoteKeys(t *testing.T) {
	t.Run("wallet not ready", func(t *testing.T) {
		s := Server{}
		_, err := s.ListKeystores(context.Background(), &empty.Empty{})
		require.ErrorContains(t, "Qrysm Wallet not initialized. Please create a new wallet.", err)
	})
	ctx := context.Background()
	w := wallet.NewWalletForWeb3Signer()
	root := make([]byte, fieldparams.RootLength)
	root[0] = 1
	bytevalue, err := hexutil.Decode("0xe0a586bb51db522c31abcbce14e6cbf6a5bbc7b3331cdb76378ca1b98acff048c11099c2713f229c349c430a6aa5623fab8d39ec266e0e7d81543fc2e4b905ec7fba75b9ab3aae53e18e2a018297ebe4bb2d0a22bd13b60b938461d922ec81dfe152224c51abcccd4105799ee2b70b53cf2401a3c01664c20ab368c4c3ccc764be5063488750f79480adcac444e274fb46500aeb2449d2a81e44c3528c70554a9ecd5b25b39550d469a43f5ec2afce668aa6598aa1c5618e569bdf08ec700a21950d6d2df3337ff196b6fcb53de94e7e127dbd7edf9c5df70c41c715b48cf4e5ab5d0e1bc4d9ead578150f98244ea47dba29af25b12a72054618d0341ebbae8e5c61cf6583c0151fdcf1323ee3cd65f8f739dc621f2aa77f8dfe36a7cef15162972c25a193bd306918deb8d6395367586ee6a534340c07caf6496dc393a0189cf81325499132a012a2b8a6152be3d3d010aadba896af83d9d447741a66100f72da46c9282a8a9af5bfec0d84d88882ce0a090147dbcef2f100f8744094a8e3712c26d875996f56a92fd99a39537197f0bbe58bb706061426e62406a300626f64b7dd813c756c159cea82a6cf82b9890be40284720b9aa9c6f1a3a78bfb8607b438ec3665225fe21370770cfdbbc20ef6525362d413ab23e85d5f6ef38a43b44874828137ab977dd9a145913ecffe2700a225042b766158d26288434511014928efdb857df4217430e18bd6c370c8327b4451611c66a118193f1155ffeef32d9b26b02d04cd083964f53b59b5ffd02789be6e8aeae4f615afb39e53f5cfbf3d9ec23640fea711fc6751abe9b3606959ac12aeb827a76a515f27d0e0f1e003e00a91a1d20b97ecde53202d6f9d61a1d4b0bd7d4f0622c2a90d67ba40f59a450191aa340bcc7b3b3107830ce1fb791a97930fd68c6b9ea0848c0591edb0a6302e0984d7f096ccb980803bbeaae3550d8996a001ecba956d3c2bb20eaa33094071e639983693f64809e449c29b59bc0b4f1530ec273f366db337bc64d95a9e26cc21ed0685cb2c606b994505bdd6237dfdb414df7eee7544f34cdf5f0e6ca1e5280b493446cb883413e26e06a00354bced7a5fd410fd92ffc39443d9e8f208aec8d81d958c060203bbe75db0cb2b982524b5e91135d4ef671ebe6c55c24bdb00d89b78c7d8fed674d1fac6d6d61bb671a996d3efe27a254e40967cb60c3c7ac5814ca5e5768f268c7002ba200da9200fc5498d07833c4b25a111d35f64cd26b108a897616d4324984e0833937344b904b964d5f292eeba6075987b5cc092bd40697ef9b2ea95cbc1eed5bf3f6337145351e98291853c3bf75eb1a533817cab5dc8d87abf034696e8dc9ad20089d79086b8608cb07101b62ae744beba3cc71e75701d46c35f317ecd1f3bb6d6078bd8cf25a55bbd200d7bfb5c9e3e2167a6abe8a6636ca82bdd63c3007b39d9a57b9f258ee4bb94325b20744087ba3a2bbda513ed067b003a0d6a4197bae5776cb25899911f92e3cdd779931e98cb11846dac49480af3c2fc3596825ccbb7a7dfa3e714d8fc809acc57577dd448e477bff03a907f8410f2ae12a9ab4a3d7738315c07f42e5af416560aafa035ae4d4b72d5e59a45dcd4c91000cd8cef454c7a157276cb2610d2d08a7bc90550c85e317fdb2d83ee26f49198dd035bbe39d6eedfbc91a1cffb5682f0410204c281f3cb8d702258c77214d77e92f1e5f4db2a6be18911c5f3950a3228d1850722f4ce0a5d56a8acf2e0311290e1334a2bfbf1251d6cb46f2a028aaa7be144f38fdb222e8a2d6320f98796731847b2449774c025a452c72dfbb9f05959c88ed86256f5fcea5458c3e22340d8ad3c3ff548f03346c55f74d6ab3aff1311a302bb8c5cd55b44528fd08ecab030c1e47385ed27de5819fd798ada8858462de7fd55aa7239e03079d976669e52ff22bf9ab6fa4860064dd5033ca6ae1fec5c628e5bc2b190ae5483514d841a25b04d127d9c536e32f3bada7b46cbb14b5718c88ed826a8c19d1fd43a7f7ff6860a88adf9fcf1415eb2c56e12a7dc6a73e24bd7cbcc7fa39ce7358f20736adc11842e72a5107bcbe78f56bb56fe403d51ea531d7d4f2681fd05f5326d7e5dd7e889a3380f9dfe8124d8f258d6f9ba6f0f6467e787d996da6310196a70f551e64d1a9dc51fe907227f43a1fb54a572db183edb726375a7a1096daeb8d7b069bf8886d282dabdb7e9101fadcdfb23c57be75a193cc3459401d14836d250b197e6ae0e4818b2bea75db388bf36f311eff18ac14b9f0fe1a354d8d397439fd202d61d545f430676eb16c6ecc4c3f583fa8767d65cdc4f3155af47629cf1b0b833a12391b02b1781f1c31cb6b05160241b1e02c5889db631ad2fa905d608b2831b45529dd7550d5ff91d4b7ae23533b1f6875b38be0f26f4479cb75579d8612ff2cfec981344598c76054584f6350d296c2436e2d43f184556d4e6208483e010ba8bcdf413d659fab3353bb8f53f085dfe24910f28b82ae382047383e81922f2d05b13d073b3fbb8042c9c1dd6a073109afffac32117a6b4162387949a9c2b21661eb321a340978b4c43dcb8ce264d6e30751c1e91551f4c2efec349bf0f083db63f3bbbbc83be7eb044b17a7fbfdeedbdd80e76580a5082d7534cea34620997ee593fc0c725a9cc41f192cdcb85d2021f2dafea48f14f63d01329845c0533210075ac3d1b674a5535d37c5c5acbd8fdee0ef9d3dc66b9fdec661f3ed53d1c70c825937716af2d44510d07876b3d52c063e7ddca41faee15d3b81940dd50d41ad5791b4b37f44cecc11db9ce58c7555491ee822e8ff1d8b0dac3eb409f8b827561ea6b7d88af82892a53ff2d239c76a8a1a717b101c7b9d7db85a84a508276b8d1ba972d31089cce5dba3ed722ead0849d336b1002f41f1b1b93d2a7e56e5c222d21327d872534aae80e8f7020c4fda6fd4765bd94df4aa38c51924b356412ae0ceff6adfdad9b9c793ee6aec73a902f658ffd6af25abf374368e38a8b9e91b34d2eeca566eb39ffa67978077870b21279afc7760f38639ad6fa152af670f25de919fcecc16755bafff466a0b8d9910bf84bc5917a33ed76fd62c47a9a2ca68055668a13f11616b7f95cda26c2b09bbe8c83609af99ec41470bda5b12524a849950caf6fb96d908dabca97187858c83a54cb2dee7fcabbc0fea8d3ca1b860d1d7b5eb1dc2a687330d2fb237f55d97fbab4694e4037355548f1c20122da77eb0b7b90205989bb9ef52f76f88770eabf56f5d9ccaf572b3eadb7c9810e93b675e7e9ea26f8d8749fcb23c63993d62406db2a53996dc053e698e70360492e2c467e1baf2d76a9dc74f23c3be3d27685c5fd07a30d2aab3f2cd7fde563e29a3434ee6a51f795a5e114a3d6259732362126da789d82ee54dae91c3e2c060a4f79943068cb6a3ee5692587a67816aa5a9c5ff3805173c72a5ad2b0ebd8588253bae50da117d938901f8ffbf725ced16a76f9f53d782ebe1f0d6f6dfdaa4fe8f93ec6246b66e561c740fc7eaf6c771659e90f545b9e89221fc9450543424f0a14ad7484253251f658e56cf1cb161b4cee63c6c5b96cf8c06e6aa524c8209205de7fdbf1e233135755ed6300ed4c096764fe4dd4855f421d272cd63150db47bc6f47bf624798")
	require.NoError(t, err)
	pubkeys := [][field_params.MLDSA87PubkeyLength]byte{bytesutil.ToBytes2592(bytevalue)}
	config := &remoteweb3signer.SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		ProvidedPublicKeys:    pubkeys,
	}
	km, err := w.InitializeKeymanager(ctx, iface.InitKeymanagerConfig{ListenForChanges: false, Web3SignerConfig: config})
	require.NoError(t, err)
	vs, err := client.NewValidatorService(ctx, &client.Config{
		Wallet: w,
		Validator: &mock.MockValidator{
			Km: km,
		},
		Web3SignerConfig: config,
	})
	require.NoError(t, err)
	s := &Server{
		walletInitialized: true,
		wallet:            w,
		validatorService:  vs,
	}

	t.Run("returns proper data with existing pub keystores", func(t *testing.T) {
		resp, err := s.DeleteRemoteKeys(context.Background(), &qrlpbservice.DeleteRemoteKeysRequest{
			Pubkeys: [][]byte{bytevalue},
		})
		expectedStatuses := []*qrlpbservice.DeletedRemoteKeysStatus{
			{
				Status:  qrlpbservice.DeletedRemoteKeysStatus_DELETED,
				Message: fmt.Sprintf("Successfully deleted pubkey: %v", hexutil.Encode(bytevalue)),
			},
		}
		require.NoError(t, err)
		for i := 0; i < len(resp.Data); i++ {
			require.DeepEqual(t, expectedStatuses[i], resp.Data[i])

		}
		expectedKeys, err := km.FetchValidatingPublicKeys(ctx)
		require.NoError(t, err)
		require.Equal(t, 0, len(expectedKeys))
	})
}
*/

func TestServer_ListFeeRecipientByPubkey(t *testing.T) {
	ctx := context.Background()
	byteval, err := hexutil.Decode("0xa375ef0deba74124a22dd4f4574ab2affc8b1383d3e81ce9e37193992ea309c25b32b675954d03d1effdf720866aa802ae6ab63eef4d3b07f1908fab4e77393089883f5e004e8f9910cb4a1f4dcef862b35011eda1224619a0a5dd71a0575e6edaa2209d0e3ac40268371be06b65b29c32ee66d0762f33dab89a98d3fac30dfeca2f3fb403289b5bcfb10d957345b6ab5b0379daf7f1fa49f4f9d4ed272ffde8e083ee818b389d968ac7098ab2c3cdecae71dc4c9b7b8621d938bde950e67fe1e2168c611682c17794c58c25def2b6d0c0c30ec0a2594116377643eab8748dee9195e89d98b017638cfec4803492038a8470cab9da0ff7f4e8cf9bc5b5a713d48a12c822816a7af6f4dd84737af34e1c3773a345c236970e6e6da0309358cbb0a2fefcc078629c6dcefb0773839b76f7984f2c12517050111296709fa46edd7a2ad9afc91b6734e524f812f7e84c0e2054bd0a0963a473210ae38ed65d6e0cc331cd7b657e67f757c92567a57d7210e61497f4bf75d457f026e2157b980f089bbeff76c75c6e2ad40a0b1fa0772fb7dc7caec457c374e121865efda2009a168af2ec262e0a2abdcab890f3d0e65b6802a08c9bccc9e8bc07eb4ad36188eb936deeb41288741001c1308ab7d087e3b573667ff074edbfbb2c13f44cfc6fee9629afe8d03e720d2a46694e8135ddd29a6e1c31376b87d8280542b76b51f169c99f833dedcd26d0bf22f164c869690d7da774f1d8b2c558f1d62ad1723dabad575a5e80d994d16ae50c6ac8b11a90ceefa720865361d4fa5fd6b8a7db26684896619ad31760a96d247b137e6ffb0f7944ade85d287beb1fcb14219d2628c4c7f173f8fe442460d8de4a490a44a55f91eaf2a9996c7268b1f3050a50a792e3ba4caecfecb9475ca52835de7a5dac4aa6b39a3cfa505ab9f5a6aa9d1f0e2cd522d67be4c96ca63989f4b51e061638ca99dd865826fcc2f283035ded49412b34cf0a08870b7e23fa190edf99f2975f77ae604001c5989498ecc329ce69263bf70565549d6389a754e60e9ddc7f504985c32d1d5ad2e87795fd0f4feaaf47e279bd66b27f673790d77cd8e5fb059d98508bb8f50bc6586ced9082e2ca7145983e2a688c4d13a8574fce5f4b36c7931cee07867016aaa1417be98521e4d306fd02eeee0b2c4525dcfadb93a56bda02a537d58518a42981209948ff101caed8cce1e9200a31541601652e27280ea64da769b812d5c6e9226f82afdfd496917b892d371735dea17e04bc3d3f1bbe58dc723a154b494c7829b449244cc5b27d986475c3309df1cd3878fd3ffae0173b00ed4e412c5b592da3237500a6f3a463bc67055e0d7ebfad7aa52d424a4e449856b404633921325fffc3296532cb3f1a1e839bbefad1215ad89992de1ced86d1195b28fb9eb32fd59a7c4c21be2e1804c2ef98b1f1514fb3aab4c523e937f976655d0d17b22f07ac42514abeae5c2e7581fa566b559a864793b6c0bb321bf95d6b55a088f66c6452f32698ec20a74a42c623eab6f633b7f7fdadec5fcd8029d9d52ba5095ba1922cca59eb53b1dd984980404a9e957182fe6b94d2573ce63099c136a47949320d8e0fdefedcb4cdda25a3bb8f46dca234162d4ff5f841f4fcafbb5a801699176425b8be28712af5d2ce6e4d1202754c6529ef2b29f0e9fa6874213a2665947a74864ed8735bbedfbd16eb950e147104d6a414c37b9d2810f97214625c97171aa83f42f948ab8b31fef51644584ec438bb3a68f0ba88b96ccfcd860424363b171810b5ea59680ca762e89ef785482a8ecdbeede9f31040d24365e9d38cfa0c194c3054f5070897bda5da8881176666f21f5a73940187f87e91c154c1f5e4d66c791d055dd0e9330967522e6acfac511642d7500e6c004bc58c9a17d8607df8b3942ccf57c85b9e6f3ae59669e3eb44c57bf45453dd93df273a5ba7e985ae283a53cbdedb562ed5cc8cab9d81ba507d63e55a398eab58a3c9789b0b6b1983380cf779598340ed3c06c8744fffc679aa29bfbcce00713637fefe21e6e4a72f52fb997ff2f7bd9cfb48de0746b0aa613fda708cafdf52b20924acacef459802dcdee4da4e059f0712c9b5e77ffdfdefc3ac46c44fa4b7ed8daaa8ec3c18b0de2acc93f405adf0a6a3e6aebda14ce3932ffc5c2d5bd36fab8d4760a697bee4859be9fcfa496c56ed20517a539ba50aef0cc23426649c14fa6e3fe475ac3be4bc7173ce64f5fb78581bd52a703e0966930d9d8cf5436e82577dae757e3ecc32097ddf3df0cc60d81ff0d3adf8ef8f4d7e274defeec687717b76f5027c939cda3148c7e21d9c73bb71c7ef8b2cb5a508c08995c4974228e53535989a0220ab945babba304851971ef9934760b0bb2bbb8dfac9462a63d15b60c77994e821c9dde88b89fa371214c30e3f7838ac3a653360e05fedf59472e63020bbdcb5e38171b35793eb7213b0dd086f04027ef6354cb66e9a99d172d63e009c532575251bf7cbf50ebbf1458c194dddd3a52d0296c804e88f5654bc2fce6eb669756410c23b975a281c0230990f8b89e3a0a64383db1e0bf0f527bb0aef2d539095374be8248378af311562bf2eb00066a1374a24c547e64cb7d23827a906a1f396afcba9afe21aaf638749b334955d699386f7984f230979fda02954b82ae8b367341a27d858d3ae0566ed0e1e66f0fadfa97240ebc6e991151a239fa6dcc8ba9c6dc9f7f34390cf90153c07f5701e31203dbc350dc08c0e3715f8698a71f975cdced652642170bf5d6566d08390e0457842c8f7ae6dd4d10c2c58f8d3207b11a48a01dc6b80a57b2a539c3824ba52ea6ec25e36733f83236c24e025e611e62a5638375e419e90dd30bbd3648302d73dcfd6ec51f0ab568f564881789650f199b69b18f8a20c5508a84f09dea451f7db55992f0bdd4298fb53e2bdf45abcef685b81f0a861ee24dbe8f5eed012a148f55ee40f0d503e35c5f812f8c9a97d98478779312c16a6c40974da6add7b7122a4795fc809e3168926645eb2dcefbbd6a246564df0a6e806222f6fc947f76f67c941124f017475ac2c6fbd5d4ef7659eadf7a0754e9ea6118f5801049895b3f7137322f4a0a856b61c0528cc5516d51eaf91727ae91ccfca0a07cf486b268f39df21c409dcb71e081fd78694042d68593f0a853fcec1956863dd246fab44e3ee10df5bf7b6634674cc8ed0ab1bc26f6f773c4583c1d5ccdb7bd94026ef0a4f5d92942712e6f9e053330807a9e6e4fe2ed197079944a8825c996f16322ebfe76a00e69cdbdd4a7c91569cb545c8fa022277d0cdef61c4b64b000775acba1787f9723b9123c95e18120e15d34e1e95e42900cc6d1233169727d80a20b86e140e6877dff2f76dfd1f344fd1069fc4aa22101fd6865bf0c5d437656a13fa3699993221a183ff2440f1b390c3f54025ee2daebed9fb86eeea8d83999c58bb1b4f6d9e665e5987e970fe1dbf7a4f64e1663dbbc3d42d1f425b242462fe1709ccee918e18d6058f0c501b4ae04a0690139c6e38c7bfcb0e32461fdd6b6d9464d18cb4ccc26672ccfafd4b615457639fb14367b120112e2a426426b9ebc80841cb385103073026382aad0dd566e14c589ce7e7476f65c44fa172d94c83110664ff7995b14bce8f95b1f1474bdc")
	require.NoError(t, err)
	recipient0, err := common.NewAddressFromString("Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9")
	require.NoError(t, err)
	recipient1, err := common.NewAddressFromString("Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000ffffffffffffffffffffffffffffffffffffffff")
	require.NoError(t, err)

	type want struct {
		QRLAddress string
	}

	tests := []struct {
		name   string
		args   *validatorserviceconfig.ProposerSettings
		want   *want
		cached *qrysmpb.FeeRecipientByPubKeyResponse
	}{
		{
			name: "ProposerSettings.ProposeConfig.FeeRecipientConfig defined for pubkey (and ProposerSettings.DefaultConfig.FeeRecipientConfig defined)",
			args: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(byteval): {
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: recipient0,
						},
					},
				},
				DefaultConfig: &validatorserviceconfig.ProposerOption{
					FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
						FeeRecipient: recipient1,
					},
				},
			},
			want: &want{
				QRLAddress: recipient0.Hex(),
			},
		},
		{
			name: "ProposerSettings.ProposeConfig.FeeRecipientConfig NOT defined for pubkey and ProposerSettings.DefaultConfig.FeeRecipientConfig defined",
			args: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{},
				DefaultConfig: &validatorserviceconfig.ProposerOption{
					FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
						FeeRecipient: recipient0,
					},
				},
			},
			want: &want{
				QRLAddress: recipient0.Hex(),
			},
		},
		{
			name: "ProposerSettings is nil and beacon node response is correct",
			args: nil,
			want: &want{
				QRLAddress: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			},
			cached: &qrysmpb.FeeRecipientByPubKeyResponse{
				FeeRecipient: recipient0.Bytes(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockValidatorClient := validatormock.NewMockValidatorClient(ctrl)

			m := &mock.MockValidator{}
			err := m.SetProposerSettings(ctx, tt.args)
			require.NoError(t, err)

			if tt.args == nil {
				mockValidatorClient.EXPECT().GetFeeRecipientByPubKey(gomock.Any(), gomock.Any()).Return(tt.cached, nil)
			}

			vs, err := client.NewValidatorService(ctx, &client.Config{
				Validator: m,
			})
			require.NoError(t, err)

			s := &Server{
				validatorService:          vs,
				beaconNodeValidatorClient: mockValidatorClient,
			}

			got, err := s.ListFeeRecipientByPubkey(ctx, &qrlpbservice.PubkeyRequest{Pubkey: byteval})
			require.NoError(t, err)

			assert.Equal(t, tt.want.QRLAddress, common.BytesToAddress(got.Data.Qrladdress).Hex())
		})
	}
}

func TestServer_ListFeeRecipientByPubKey_BeaconNodeError(t *testing.T) {
	ctx := context.Background()
	byteval, err := hexutil.Decode("0xe392156668516380838b1891caf4acd1a1dd8c3a64ca9b9bd2b0805b41b562544ae947f5e633183ea78f0ce4c1b9576e49b0066e6a406934e18e7b8e0792ffaa0df082ba240a8969dcfa978ed6b2b55af5b0afe0e8d40efeab49e7dae9590e719ad18b872c72fc1fdcc0bf4ea660018ef3c8c9c406af595348d2ecec9f6fd8e29b4f41650550dde6a7670296f4a5811dafe6890a62fe1d6c2d530f3c6fd7ad653a5c1cb1039312da835b9ab7076bd4ec813fa1085de34c8e2ca61e4ab4c5c082a3a3e43eb6316c9a9ea032169b752c42546bcca207b5ba8eac13e1e6ebb69f2eaed338a305717bc4bbcd1d078fe4342c160443e13c6fb14d9620713d39add8319acb3655df7e44697af46414cd0933558c553f0a9b1fe30c12794d237ddbd3183d991e3dd66c04dac4baad4c71e995c8741e13bc2d3ac08eebfe98bc8e4cbd434be5e489107c01078ba3e64d44967654607f29e17e96e962439ca98e02c8d4b96aed837112e85afbd8d043b1f755782c47d5e983d99168266402f394bed2646c0df060d6d8c2a0c2ea1a95ccba6d541c15542e8a9a4014bec2827334f651497f02a33cf649934bf29ddb2540dff8d98d856801e9bbba1d9ae2d22d4e7264c4e310dd677bd14deaea13a91104dce34f3075136496a1e3f605c10b57ebb9fb713ef66917dd6cc6cbd85589c5b247c042da86814e907d7cfdc4caaee3975f4a9ddddd5560c8758b0b7828af5ceab0b593eee27e927767c2023cd6cc30c1be8cef13767e4dfe4a49c1e1c967b8cbde42215af0c85e23bcdb5e8be71bbf525fbc3cef3c18e2452b5eb5954aee3ace1dbe797593425274abd7e9cf4dc8a3c581ba27077ba724a13c66a3c0120b81486d0dd8c7dcc26f7df0ad38b27dd6ce7b022f06ba522a2f9da0f3bbcce099a14f1a8a944e2c6165745a18541ac78214317942a5badf072f85e7bddb44e14f08c3d638d85e19be49d931b908d1aba2b176709435e9a3eeaef94b275b0b2c8331454f088994d21288bf00af038cb8a98423a31bb0e25753c1dcbee4fda131ecb4e0cfb99e3d48fe7ca873a67bcb5f3615c2602fa81e97520e8c9d40ff52fd761fea60dc549d115f4222b54f40f1fdaafa88c2e2a88d43cd955b41aae6c9b645e9a532801ea73ac33d8cf457caf53d038194a5ae081c09431edc2a6b78cd7f8f7f06323c57ccedca9654d9ec079fb7e90aa752aa2c6844eb8be6b7d0189a28d12f3712c16caf9660eea9ae0f5842d5414128126ecb86483ed8efd4b381334cbc3da5d758be923e8c80bd09d39f68a01b370e70f32ad38173e8981f7174a65c9063e137c150f7b851c9b6e49115b227bc11783b1e276f8a70e8dcbc7fe269f27bded826307ba18e5b43b14b8291c789ecb73627ab1976f7345a4751f13c667431be8657fe20e70fe297ebd5d863967c48cbf45ce6110067d75a7e28e3d054fa685e8f7685cdb91954cab8271aded2d11be370a7fa12d6862b948cf23c24acfeedc2c11c8f39a70e4d62e262af4abf9f255eb69e513749d04cdae9e290cdb6bb317750cd53d3a63a2ef0914cfb4b84fa5ef08ede8da4311b4881aa380d022a864df0f38b31b98c15bda2dbbb222c5333f3358885da59cd5fa72c169f1aa8596efab08399958fd3663da988d8eb084337496074f8d7a6a86db2ca57bd8a9952b73980e1acce37fafe503bc9e37d6bc457de85cd5994aa4d57baac436b7d755d130641668dc4de2df9290376a3985270504cf3b1ad5aad15bc852e81a83ac76b55e523421ace61b82e6d760d65d4909254203b9d97e2beb7fe4708af073bb204361da09255c53fc701d17eb8245e893ff629ab5f9ff433ad7a28631124468981ff8508aeea11ea1d589609702e759338f7087f5b6c420fd27822b6301f54d2ff6b1075792751a952d90c0e29e26d1c39d4a6524eb6dd8d8280edd37d3bccc29781323dc25c9d706f4fe3144ec1c11d0794f5a83715d10de94db7a066d723f5b3ad9b9929e2bd7711b5819844b6b0f8397478fc029854bfb31886cff305e2b5378d2e6e1873d3992102e66e4dc93e35d4cb4dab6257ec1ee24f59dd8ba1b09dda23d20e40687557f1fb284486ea9734929a101d56c52c8b62054c2c15de15a113ffdc50b8070a96006b987efafc818ca5f5bc40f183beacd95b6cde3f68c279770144480acd8649e01cfe722ce5f845b5c72389044fdef152afb2f161328fe2542ce16512bf61ec96bf70c5d9036bb853b0071f6fdab6a7e291a0c37606da48a8b6ac7035f9285df8d132b0618dd6b832afefcf21bb242769ac85cd1aa6291cbc7fbfb9317dcd7e0398dc323ed15a8d62b76a4b909a76b10133ebaddef1f7dafb7c6827066c7fa9de8aa50fbc6a9bb14c67a1ff01d6bf5c1748dd4230e1499187633e6b2528cc43fe3486483a91b74a86cb5548b406acd118311bc33fe3fc63873863023d75aa48ad73a545b102d413ca83448dc15f370539d0defe60c624d127799e57a0b32feffb27920abd98677c54e379bad0960f180c26cacfde635d0462c30f3ea6df2544f89fec391dec0e3f7a66e3ec2e78f9653c0b154a5ada3d8a64a67e124530ab7868a0d5cc9faa3abc0d71e0041d0ad569585f09a33543422180660bc6b249a325be73cfffc63c17b90ac48320767fbdc98dfa72143e55e564313c7195c8707534c88dd4f7d04fbb2f67ed58d836d25ab801d7990800642e6a4e5238090c038396091c568eb4d18459e7610d2b4462ced87eea54c15adf2472c884d8db969704366c88d1a6f0c927d30bda1e4e02d28d3cb9487ba26b447c190ef4c1c91dfb89fcf601df06cf425dcb8a41ba9c546073f4c4c9d8f0eb3efca8dd7855d013b085d2d641c1e13386d2fd1c7b8d4b3e8740d978cb65f4a5cbb6bc696c902004643ef760b2d1eb7748db10f4e25d7f487dfbc33572e55333b5d57a73fcf8d4f4adcd2955bcd0fc82944864252d20c729ef73577dc662ec6924a590f619673e39266709e4e6a7be6331e9383914a24fe86cfb0f8b3b737e5b994b1e9aefd2879333ad3ec6f2eefadda01e0cb3f3c3619db1004b62624cd58b3383dc255399829d79dc020c551d4c1d95cf02eb98e5c987216f62d923d47d00ea4cd6b813cb551c916ce85c36c6f0d2afe0bcfbbad85a5b070ad8527292411db6e7b3847d28e25168f19f77c9d930cf827666ddd2de4f31f8d1202c60edf4e7b0129f051698e64c4e4d8b6ca9c312fa0455233192b8578878093ad286299025e512690ee41bbb7cfa5ad93cac0b0bfac741578580dabd9a1ed92538112f112a1ece6c59ca0374c6f3500099a2045b0ae32ebdaad5f64288fd44c10fab04b17cf622da944de2a87fbcc4d22ee21b1357a8dd7345b48e67f35768da1a8984d634dbb56855f5a63fc5825e38b3a3350b047c8523621bc0200bbe1e779e533f9da39136bd97d376c491d60eed1e5a7e266d63461d8d7279582592d57f66f639e097ec14ede36d87a08aa8e9c5f18030c0a2b7b9d8bbd49627fec8b22071f01fb29feafe774b85821dbe27e31a4f3055cb6f2e67353803e27bc828f047da64bf33b3085f91769bf57a974f4cd8c24c632e2fb1e9fc35578ae3de3cdd31bb916ac629769337a445ef408510ed3323b390fcf9f09b84d3d6ba9815b7d216f9")
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	mockValidatorClient := validatormock.NewMockValidatorClient(ctrl)

	mockValidatorClient.EXPECT().GetFeeRecipientByPubKey(gomock.Any(), gomock.Any()).Return(nil, errors.New("custom error"))

	vs, err := client.NewValidatorService(ctx, &client.Config{
		Validator: &mock.MockValidator{},
	})
	require.NoError(t, err)

	s := &Server{
		validatorService:          vs,
		beaconNodeValidatorClient: mockValidatorClient,
	}

	_, err = s.ListFeeRecipientByPubkey(ctx, &qrlpbservice.PubkeyRequest{Pubkey: byteval})
	require.ErrorContains(t, "Failed to retrieve default fee recipient from beacon node", err)
}

func TestServer_ListFeeRecipientByPubKey_NoFeeRecipientSet(t *testing.T) {
	ctx := context.Background()
	byteval, err := hexutil.Decode("0x30e227160d44bcd35b0b2c29e59dec2c24317748224617b9b7208b456b75246ccbdc98860a4c712d01b6d8198a4496c1642393cdf6de720c37b423adebe63dc739cba4bd83b6ef31a2fb0dff7f98a20efa9372e39dbcf1ce8e0288c4696ad9f4f511c22cb183b8c388d4342138303318c2fd6692b062230d4e957deeb47d31b83175bcc58d10671c2a026224315d10830caf331d184cf60764c813e40390b31de95365d7816811991beeabb176e6e80e6f1e80ddf4efed95d3d43e9a7b98cc094e10d4bd16b2578c6e7073915ecdb6c77661f83d249b25ca4f4edb5808f7341608c6ec6ca6005cca3753747cf63cfa6ac18ac7cbe4277e6c9ec2145567c380cdc47d7522f1759940c4378e3f35da60bae645b76c098ade384326382a7dadc8cea365640ff70bf98f2518ad7bdcf354cf35453a24dec28adea08516e9222859d0197094ea2110afb500137b2802d58b311eb68b849289f95068d851ec58d53cbd2d4422c6661ca5c2b2ab6f69d45d68ea1e7fc9152eacf1932d59c75f1f66eae7c30c05ad9337b7111972dd3faf577438d7b4c1f6595e52ff88c836198e791854954e5b90fff6e3597d154f55c2501c3a370a9f60579f6b681092c581b734e25efa5ef2cbb455bf1efc5490f0742065fe466a8fb897cd3099bc43ef9bbdf463a75afff5efc3fb1d3aa8526291f711120f875710e707cd6fb4028ee4473e697b46c521205902fa89e84209806724bbe7348c7e4bf1b59c33de54537f6a725959f6a36ee63f48403a8e0fe817aad33c3ff3d9e796cb84f0658845a8d129b36cefd9c6d3ad240bd49e60efa495f5ab13faebe630773f4d5be1e88a61faa93972a341bc715b3301eaf472f2084a049e174658b7a7d973759cd75225c3aa94aed8d53f3712954158bdba03589fb9b079a5d7e4afed961fbd8ec1b2dd69bc04385ee444a52752f070ff4cf3965cb4a29ae6fd9ed092e18396ee686aa9b2b41a6ea8f22f54d5229e0971fe67a768ac3c2864a218c45ce41ffd7b785a10bba5795db0e0f9451bea25d530c574288aa9347cf74cb26596e2c41eb9159cfeedb6da3c2e4e1dde4c2ada1c6ea6c9831cbdff6cc5aaff21796d6612b8e76777631bab13b92e27f42db4cb01ad46ef7318e96a18b9c9d6b4dec1f82106e76772e622ed9a4fc5fee9c54356956aac3cc6847d1d92983d4fcf61cfbf32d0ddbd0fd061dca5362dd6f43af816cabbe7636b90773bd17d136a7453f07d64328a9c322f2ef44519323127e817acfa2310c9985538cad67128151927c6b65a33f725287c5eedeaf454f9d0a77474882c917d495f53d2feba16921a26efcaa4ce45c5c1a9840d032fe8d7617c7b2b7e96c292d620d9d7923eab4cee058cd1676bd203c9a5f6aeae4085abee97f73b0de509d97c26084416f45d94ec0d1657507e284e617b725922a24a1acb5d723148d0bd74bf45be8f63a7f708e9125999451dc51b47c6a4ef9c55adf1a37d38559a0fd3becc64928bc031837ac9cc4fb32054f13509aea7b7abc76f84f4e591fdf9c275cf7cb84705151ac9e3e46d740231bfdbfdab008a14cecc4d39fc056dd9bb3c3566501a0a5e31454359f927a03ee2a486537a5ed7bdde6f79bea5965b927a17e5a7d7932546b996861a5d516ceae9c09c0bc49ebb0839766c68f0283d0505a3d2ec1e670f25f750d40447d8a8754d0387af0a0252802edfc83b0596db723d13800e1de5f5fed4151f36cd46cc5f96a00a110279a180b3dcdb6c7fc03f3f057da8f7c6cd17886c78a1b8d10f8d9df7f6ac3d5fb180cc99b923c430ec2924650319ba8bb56e6821933508dc83d82fbcb77769e328e3c1a1806f9454b7fe51c9ca05602bfcf2e52e467af87c82e5ae454548cb9e47957140c27669e6876a59dec7bfa288f0f8a33d9feaebbfa04ba5eb2ef3c5b6289e98263d49480557afef45af8262e1b9e63a667a746265c5b703554c1038913c6e42211483f6899b5c2b7e9145817e60b4c20efad78c0901d75f34b5f66ddda98426b8c4a71d448722bf5c5bf034da4d09bf68eedc76eb2d261466d29c11bfacd6efc2bbd14854dec0a3b2eef862e741fa876f45c8e7dae6c4bbdec53fee6b38173b3535a12570d3a9dfa1ebfe4f6b7f6aa48811529963b323d53569f515f1c8e5cba1c24256cde511c9eef4453caa193b08b440e81dc49596958112b1800771d6d52a229fb87904d77e6505a185a97b099318aeca216906d1c1691984757db6c912f329d026ec444348c6171b14e0a251af3c58f394996101f0bd811289fdf4c16f4ff099635bc697025019174e028b59c7fe81a6851b133c4880919c42882989dafbd9b641d74e120b1ac479c5086257069809636882d0814771b6c651d253901949a36372ed608bc2b688e45b08a5cc422fe76c9952b7aaa3db443e3b34fe7aa8ec75fcf6da233639cce4374800d6b63c978e9afffbd8436a6d205a51e2228e094cc01264460ff23da1b4641044b1577ac046469088778299b3c390c86fa010afe4e55664f8b9aa354c1aa3648102f7b1a9401d9b6abc5fcc48b0f1f0aa53916e34995c22daeb18ec6d70cbf9b27ffe30865214eafecd443108103ad56ed6e3b05bb6f250b110396f688598fb6c08b76c422fdce75bd59e2eaf47dfdc4e1b0e91059f75e32bd473c12011e72b78b6a1bbeacdf867b8db283f1ea53d47710e0108475f64f3e12ae0e2bed66f1e963bc311fbf8cef898e92ba7ae644f5f76599f7967f9c28ee94677c4f3207512f3f114bace4df0b15e2536e0634bb8778df97d7cf5b23e2a4175f56c8a54d6a0ddb178f0bfb7f77a384410e18773b31de015908b76c5cd5c4ec850d50c6b9a344e8fbe204d41a975cc35dd25ca6416cd50ee2690d47653c28bb26a9f395259c106597904d5d8864c23a43ac6067120b893c3a0fb05b672e3366df10a14df3ea6b54a7528160eb07ccfce6c28278c0d0e360a0b6e5b981b98c9bf150ebca17d097a7a5736dc8b1658936a577f71b20991be3988f24942167219c13f46757761d5aeb1a2a41a8ddc2d122b5c1813907fc89fd0c6e43ef81576563789f6122a468ba9dbe77f3945d5872cd4c9de7ac09f3da85d8941c36188d7ec79d94237e16957c561b06469c1c9887be0e9f79ffc37ef8f2d18c331f45f1eafec048bd2e5e8791f5541b506116ab913c06a3b6b020496bb01f891d03b6a8f2726936bfe921a6caff8f17f354c5c5985718f6f9d22cb1e2bd51e6259a5b160559b9f1c3e7eca94a8aae6408f7ff6ad4ea64cafb617a049617efa6c1248d88dbb327e3bf07b63196fa48d2e973f9ebba09b329b481f2a2b31104ab1032662bd93ec7015b6b2c36d71befb5e05e9e0918ff3626b4317d7a2ccd1cf2c68e0e383783e2c0294ed2c51cd6fd5e2a4f4844b4a342bca3384f72db3560463f100ae4d17678b3cf6bed0d66c82e4b37ed499a50bd1f0ab436c0d2842a27293aaecbf0a647715c646be232df2841a49198fab72302a7406efa61f47a2adf89a34b2d152d6a0b03e36c78696ea844186fd9e59c5288dc4f6682a4255c66ff152d79a7c404a7f97bca5bb90aefa2e6259d04b26a2881c76f3444c5a09c611c13f8293aba60001e713130268896cebb9afeb9547f6393f398854b75f20d7666c6eb167e9d3")
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	mockValidatorClient := validatormock.NewMockValidatorClient(ctrl)

	mockValidatorClient.EXPECT().GetFeeRecipientByPubKey(gomock.Any(), gomock.Any()).Return(nil, nil)

	vs, err := client.NewValidatorService(ctx, &client.Config{
		Validator: &mock.MockValidator{},
	})
	require.NoError(t, err)

	s := &Server{
		validatorService:          vs,
		beaconNodeValidatorClient: mockValidatorClient,
	}

	_, err = s.ListFeeRecipientByPubkey(ctx, &qrlpbservice.PubkeyRequest{Pubkey: byteval})
	require.ErrorContains(t, "No fee recipient set", err)
}

func TestServer_ListFeeRecipientByPubkey_ValidatorServiceNil(t *testing.T) {
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})

	s := &Server{}

	_, err := s.ListFeeRecipientByPubkey(ctx, nil)
	require.ErrorContains(t, "Validator service not ready", err)
}

func TestServer_ListFeeRecipientByPubkey_InvalidPubKey(t *testing.T) {
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})
	s := &Server{
		validatorService: &client.ValidatorService{},
	}

	req := &qrlpbservice.PubkeyRequest{
		Pubkey: []byte{},
	}

	_, err := s.ListFeeRecipientByPubkey(ctx, req)
	require.ErrorContains(t, "not a valid ml-dsa-87 public key", err)
}

func TestServer_FeeRecipientByPubkey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	beaconClient := validatormock.NewMockValidatorClient(ctrl)
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})

	byteval, err := hexutil.Decode("0x5f46bd9fff82e674ed1cd124d4b4339135fb5d8d120095e9187bb62ef160f198d9d4e76a010487f5cf30aa5e152de3de9dc35b72956c5a837e6506e5d5a4dc1d254568c983fddbb7f3696e3e25d622a114f1d7d1475d0a998647e937a379946b95e276c878c8ebc507855dbfdb27194172febb008926d7bef2994e64730b4089e7884fc19929b093d6b14592b459dc2c1072c62e011aa1bb2e4f11e19fe79b260c569c568912f0c515cf4957f3a7e230fead99956f06876ff345602c42f248fc73e6291b0da78d9af10d031b00f4af675d901c70eeea8b3c71f7d01bad01b7323bab19dd2e28dc3b5b08d0b229faa3e240e9779423975d373fd0f072a4b605db0b72749525f7627f4fadce1b9d22a3cf4193ea274c6fb43874662d663517a28abdd749e030013852d821f00bb517c9b77822fc715430d94777bc1a99f39ee1493c7ba7aab41801f5f6f592e331662aaa732381f23402fe6dba7c79c122c5af01380a0032ecd68386055eb62a57bd7b97c5b003aa4920fcaccb0fc8e5297797b006157217514adb45dbfe5fc479c0f386213486f35b8751ae8b8fcf1546cd8ee8222370526194c0624a0ec3f7bc64659f73c977e3ae667e88fa0e992a7879a5f88f4f322dcf967aae86771c43271a04a4387ca81bf104166494ff877165199fd1badcb99523879b44842d67d1a2e677ae63c8f82e39aa350a00b6bbe616e6eb77f713b09d2ff34e2219c3e1993f40cdd10b72b96d73733e77ef8aa84bd41d2a40f24c1b9774cb90c687069c8b7f2210926cb0be85d8ce455f54d044ec887128d376a6760662c7e8c99f5016a14d17dc6f81722009fdcc86fcf402cd5084ab1d4f4f6e1cf4c1fe630fca843d4343e2be5af12b60b58a70a3bdbeed61862daccbdf1ccbd6b0eeb077c761676f504cf0fb6653fef00577f6d88ae74cb25d27e52beaa534659296f5521ed1f1c793ccbac59bbc563467cfed750c29d8086411517f8c59cb507d2ddd6ea9acead5cd71a2f07bf374fa93644a4178286fc9afd49f9a15f7aab9698f6b27ce41c7f1b41313a17c835ae7ebb6c9ff77c48f404e4796ccd2b3ddbcbf86a913ce7d47df7420cf9446d31665e65b83fa70b7ba3406ec70ef520ad726ded070e749abef459fd749e4a162eefe3a10758e8ef66a3fd6b51d7615043c5e24014ea6fbb3ebac4a52db2df0af35e108db6aac6a87bbc36ad91acc5be4606aaee716059410d01fb7f8085cae6cf85c49d56c39704c98f49b0a938c97c7595adcc2b29263b2a7c48536310351a03aa1d48535bde47651dca32b7272eec869f9c0cc7496ba054b0d158c49d3d3b3e3ed62c918e382e6873aa4aef3a68cefec7617c18918552b6ae488b3f21321acec1ff3af4e21e2f014a531dd7aac918572ccf6d1ffe4e1d24e123e0305684c43de8d11a5b0a079a541cb945d3ed8013712d613e210255f31eecfb75ad4a923c711dbc04e118887278587c50a1410d754a600c55fc7d5d4e96e1e1941f6554aeed1139a196398d028100042da98f741a12a80e068cc32f9bcf2fb363d2f935cf94e2040bccf83cd06564d2436209b53d4c27c8f8cb3b31ddaaef0b958b81113038e889e6d758021580fb3eb6f915ad0448ba9e1e9738b63ac53e1136dd9f6b1f8d19a0fde8be1cb1e05475b1b56b8240809156cf984c630cf2962149c7a0fbc349047cdd9488c92ed23fe7c6f66ce63018561f7a7244ae88700835668e15a12ec1b1d5db6a23fa9a67d3b475cca58bfc2160d93f185bed5ef17b43af2273dd08283d86dcec3fae763fe6d205620907b9a6a79ada1ab87f5c3781a038e2788466dda4e8a4c5831e778ad6b67e33cddd07f5d71469086060cd66bd7f5a32ebc40e39bc48d4f4467541b7fae77d954b4347ef4539c32f13335eca3215604e9ff25858bf66f89fa3b7befea89877290e0137ed66c8cdf0aa3b5369f9291073d89e3b13e25362e82ea7209bdee790d2fcfe285cf61fd4c6d6ce3d1135d2df02811b4ef87d0d441868623b79c6a0c90e6a0dc96ebaa6eccdcd41d9bc06057a03863e1bac74af6e0047d7546343913f5767aa24ef7d5fa0c7ea28fc42b14dff80563c2d27049e472a76bb74a67c732f52c2ab8623d2e9c443f9002da22ec7216530d0cac86d8775d47273cdec8f7014dd84a42590069b3109fe763539aa001249bbedf251f4f73aaee050406529aeffac8205a797c8b14217a7ccda4de9ed4140aca79bf412894e00a265a77b2a4a7e9ecb0ff7d2d1904eebda8297ca4b13104a4938b0f31e3ab9ae254b6d08bb6c8b686b86c31ed2645b1b418559b5120b812fbea30bc6f222e4edae19866baa97e6be0d55d62b01a70bff972cb467493a340dfd702f989fe36377d02a592214dde3600b4b167377eac3c448e25045241d8b5a60a29e252301dd800c0306f4bc4bdaa70163b840797a121a1753c1a900ff3e099fd6fda3808d6f5b7471df566479b3a6de7aa887352a8736ab233c827db5bc8bd4653e289811cdc9f1b1dda36fb932af335674f3b149f2dec8401c99db795c9d35c6c4dc316434f43f2d54028fedf559dfca104cf5e00498387d0cb647110420684386759ed64c45c819aa3f418d5473df5e21120ec37bb600be6786e302dd906bb2298f652637c3adc3e5da2b8753b1f2b8c12f15a29a47b63bb181cfe493f65ffdcbce1e719d3469b6f5df1994253acd29501b90dcb88d278c010279fcafdbf11c35c5e3f505e8a2bec030efae58f6e18e14c6f313d02831cd8a0b266116b5dd5e6e58171b0d4cf4dc3818e5831468dd9c53974bef34b6f1d836e8a20c2cefd25a6cfd2dc0ec929341c0bbce1365b37031db3841e4f8345520daa96a1fad8bffd981dd3c2da438e79ce4efd3e53b2d35e627d7538a3273fe2f6de7cf12bd896201b06cfe1634c1a0fc32fdb1b3093458cfd5d9d6d7347a450838c1d989525e9fa18dc921c4fc3fe41c6005654e6723600a57f5d040b44b134a70c71ebcd3793b21c1f6653bbb3533f7f13e4a87e0fa32ecd23128d9142ef48669ad872870d5a3a3dc72f70ceba103c0f4e5cb241243bed9b6618cec9616ed1c32291a1f3dc834873766cc8179f22c1d9dcca647728ec369b4126fa0f198723124fafeb07a947f6ab0dcc66b6627b4db032e347da9abbf86b2128140336ae3728bdf99a6c1eac8d4ce042d0374d4876f9c36bb8a025746ef04181b9f58d37dd9e1dafd199a1bb63d8e4b0a8485b6e8cf399cdf24320c28699c6307f5f348d7d8bed7b4512fc9ef39db6d166bee09fa86c3c68ddfb6dfca35c9ea2b2376f65d3a21d6e8bfd305e6fdf7c2f538204a47646373d05833ccc8e269dde545b403cf304f030d277f3678d50ead958acc26079fbb9c6de2e5ed12030234c7f2622e4e708f84e13a3d8f9ad69bbb7935f80f85d1efe68f34b6739d7a39f5d6a936577dd929fa9b5337af045f6b9bbf0951481c56050af8d2ae6eea660f6178ba85716834257261c16cbc0def8838d26d9507028f52f4c80fac01ef7d426c6a020bd45144ecf0a377982ec607c3b90cacc29c28163ed66dea58d761b85841c41f635fc254adc3fc7223cc2d7f82c39674c5ec2346232229c8a0d5596e439cf532042f157f7d849146003079b1c37dd7fdd669949230fb5d6f5c4d21b2d")
	require.NoError(t, err)

	type want struct {
		valQRLAddress string
		// defaultEthaddress string
	}
	type beaconResp struct {
		resp  *qrysmpb.FeeRecipientByPubKeyResponse
		error error
	}
	tests := []struct {
		name             string
		args             string
		proposerSettings *validatorserviceconfig.ProposerSettings
		want             *want
		wantErr          bool
		beaconReturn     *beaconResp
	}{
		{
			name:             "ProposerSetting is nil",
			args:             "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			proposerSettings: nil,
			want: &want{
				valQRLAddress: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			},
			wantErr: false,
			beaconReturn: &beaconResp{
				resp:  nil,
				error: nil,
			},
		},
		{
			name: "ProposerSetting.ProposeConfig is nil",
			args: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: nil,
			},
			want: &want{
				valQRLAddress: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			},
			wantErr: false,
			beaconReturn: &beaconResp{
				resp:  nil,
				error: nil,
			},
		},
		{
			name: "ProposerSetting.ProposeConfig is nil AND ProposerSetting.Defaultconfig is defined",
			args: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: nil,
				DefaultConfig: &validatorserviceconfig.ProposerOption{},
			},
			want: &want{
				valQRLAddress: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			},
			wantErr: false,
			beaconReturn: &beaconResp{
				resp:  nil,
				error: nil,
			},
		},
		{
			name: "ProposerSetting.ProposeConfig is defined for pubkey",
			args: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(byteval): {},
				},
			},
			want: &want{
				valQRLAddress: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			},
			wantErr: false,
			beaconReturn: &beaconResp{
				resp:  nil,
				error: nil,
			},
		},
		{
			name: "ProposerSetting.ProposeConfig not defined for pubkey",
			args: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{},
			},
			want: &want{
				valQRLAddress: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			},
			wantErr: false,
			beaconReturn: &beaconResp{
				resp:  nil,
				error: nil,
			},
		},
		{
			name: "ProposerSetting.ProposeConfig is nil for pubkey",
			args: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(byteval): nil,
				},
			},
			want: &want{
				valQRLAddress: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			},
			wantErr: false,
			beaconReturn: &beaconResp{
				resp:  nil,
				error: nil,
			},
		},
		{
			name: "ProposerSetting.ProposeConfig is nil for pubkey AND DefaultConfig is not nil",
			args: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(byteval): nil,
				},
				DefaultConfig: &validatorserviceconfig.ProposerOption{},
			},
			want: &want{
				valQRLAddress: "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9",
			},
			wantErr: false,
			beaconReturn: &beaconResp{
				resp:  nil,
				error: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mock.MockValidator{}
			err := m.SetProposerSettings(ctx, tt.proposerSettings)
			require.NoError(t, err)
			validatorDB := dbtest.SetupDB(t, [][field_params.MLDSA87PubkeyLength]byte{})

			// save a default here
			vs, err := client.NewValidatorService(ctx, &client.Config{
				Validator: m,
				ValDB:     validatorDB,
			})
			require.NoError(t, err)
			s := &Server{
				validatorService:          vs,
				beaconNodeValidatorClient: beaconClient,
				valDB:                     validatorDB,
			}

			qrlAddr, err := common.NewAddressFromString(tt.args)
			require.NoError(t, err)
			_, err = s.SetFeeRecipientByPubkey(ctx, &qrlpbservice.SetFeeRecipientByPubkeyRequest{Pubkey: byteval, Qrladdress: qrlAddr.Bytes()})
			require.NoError(t, err)

			assert.Equal(t, tt.want.valQRLAddress, s.validatorService.ProposerSettings().ProposeConfig[bytesutil.ToBytes2592(byteval)].FeeRecipientConfig.FeeRecipient.Hex())
		})
	}
}

func TestServer_SetFeeRecipientByPubkey_ValidatorServiceNil(t *testing.T) {
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})

	s := &Server{}

	_, err := s.SetFeeRecipientByPubkey(ctx, nil)
	require.ErrorContains(t, "Validator service not ready", err)
}

func TestServer_SetFeeRecipientByPubkey_InvalidPubKey(t *testing.T) {
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})
	s := &Server{
		validatorService: &client.ValidatorService{},
	}

	req := &qrlpbservice.SetFeeRecipientByPubkeyRequest{
		Pubkey: []byte{},
	}

	_, err := s.SetFeeRecipientByPubkey(ctx, req)
	require.ErrorContains(t, "not a valid ml-dsa-87 public key", err)
}

func TestServer_SetGasLimit_InvalidFeeRecipient(t *testing.T) {
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})

	byteval, err := hexutil.Decode("0x313fe89013947d9bc55c4a39489ac1dbab92fe6b73e49e090d2fe42ca95bdc673245f0fb517f969350183cfae4f9129240d9c20933b9edc237bcd74f99f1d3f2a63590cd8b6fb0b919e8929aaa91da90765716dbf09f630c388497d1f5ba2b3ae9e9f5b9aa72b9ddd5038be01d3c5504e034ecb45a7e29c1dc31a31bdb71b0a36fe8c58b15eeee156df71199d1e08169559c0933450ace7f5e31a419c0b7c792df4890a44d11bc91b5fb5a332d65e1fa733a15ceb7a9fb7d455ec266ea5526725f9ac9dfa7b30dfc56163ff88e7822960fab828ad745e84a27f14df65d6543a02181ec9a1b9764d51c2319fe6c86e273c821a2d94c73c51385e2e21b10ef93b966f3db85d78cf3a62c678eca937f2f5016ae909f10692c8d311fd42709991afe59ebf924ae059145c17838bdd301c57a0cf3fa2b61c90d0a927c92942684a334f5be53a23d7052cd3361a86a32229addd3acbc74e2111b9013d9ee764412aa07a485bb6fe6138f59411218ab2e13fb58c1ac3facc2851bb0486b236bae7f56900a88e5cf97098a3e37d0c7eb89fa47e25881288945326190a27b84ae1f78e9e72b5cbc5be50beb4fe9c25352eb25059fa10aea891d2e7c3495bb64b3c8ee8c39b724d463c879060e78c1cfd07a4d737ce49302a6bc003e5492725a544ac1a541ab029767c559489c2984dd8be0e2b132e59ba2656d69e2f84c28d5c2d800195fa72bd48117d0bc03d13ef782915eee6998e643d3e11b978dd275a0e340c22c716b4523f9851fb125dc54dc4ad88a30d45d61b38edbc4055e650bc5a5e4a3992e89cd04a499b7ab20eb0f17f85b2e8048a981c764d39549ac7b0a8e886efc3c88278be62c8128e9411c22f8429e00898c371442ad8de0fcb0364f36d621e9512f7128ebc34854d1768d3cec30b3f70a103d2e65f9c47d922615390d18e68362b390c7ab296f4f526fcbbaa1cda6ce5375155e539545709e7f9048cfb58f3b7ddb58e7e42b28a0306944215ff507e620d0ff57e705c3554ed33c2099bac0185798954247a52cd32bc9080f948c64ae39bfef5b297d9508059f422c7fd93f474ca1d963eea44a1d74c50e04c9678001c8de26b8d9f5227a54aa469eaa267f0a47c20ad871f15b829d16c7ac0e351c1f9b98490e44da3cbb947554e59a039ca70763f9f8e6802fb603c366dd8ecaa338649dac248aa3b2a8667591ed6fe08c3f259193d2c5cd3b68494e344678f79250f78294bb9f50ab18697532d62652925b3dbb5dcb54b952f4cb78b37bb5f9664fc22a16fd223a8b2b18cf929c8f9b710b742712fb90926d7141c9dbae6ad52f03d969b3a312974877cd044c76720dd8fd66e43d349ff759ee402ac13c31f5dea840defea020e52f92867aea68ab5c52303421ca87112665def7bb97db659e8381e94b77ea359da080a271b328b30893515fd63e3d78e65d4344ba387d58eca62b13195064a6785f4fa681e4d59ce5fa01acdf9be80df43e72256a8c9689bd040306050ef8fd9335311fb8b6ccc6c5a496c59a3484617f45aed15b664eef7fbb654029480daed36f5ff3b157a0c23e6df26630de3a0e0fe65c136ef52250ca29b31c1f2cf22b049330666d4637d5992e1d3a8c22f1deeacd827302e063f6e44d3653ccc595f7b76653c1b4043dcf1599e3739a541cc65243f9970e34eb39b3dded6d85d5a55d6dbb33537f85374cbab62b75a00237a0b452a727aa37baa834f95627246683f0ec5142e8a208d2575917515ba8e2e850baef93ac8c566edc51e12c04d459cea4b7cfa61cd9e67ea6be33a049fdb60bf7e02088bcf2a45d7a6f608641dcc83a8273e4646d289a5c94a2b081e9617575f8389e4e70bb6c086769843e118b3b90b54ff4c7ff5e0215346fb34b90fe42fb4a6d0398011e26691ab1680c113705dd718c6ec24b723920203c9bfde489c0dc4bc57445c69b659fef14fb53b0b66e544da7adaf7731fedacc512b26618f814c43e330fffc768e8e330c0c4fb0cea44bba5a7e00b544024e0bfbc86736b217a97d444044df526b02c615d828c0bbd7449066bab943cb35dd7362d907f893ba106a3bf551224b9b6ce734575c9db4d8602b4653843ecd73393ce42c0db4726eec116c054a23428f9d4b017703f656d39242987cefed9e0bb13e77184d33897483041ed42e47ce390c5bc5fad67d515a34b335a495064be9032c84182e6707456a8a748c984e0c12f96f138e5cf7e33eb454b82bcdec18383a770477ac67d274498614e43d103622e1a98b78425fa7506b1a252a317f5339065c3e4ad63c141d37e7fad490b099c9d4b05220ba2e55adade32490c29b95b9d5c995ad91c890702383e492d987f2d40306b9ad77122655ffa7e8dd3c4b01ce865dfafd0ad62331542b48c41d0a75ed21919171b6910b48bf2bf5923cd76f36a5b3920a5465345281bdddfa462ef01780a0622dd9187cededd43e24821208587a4da2ecfb691047e38ee7843052497213e40bf93477e80a94cc628ee12ec3932da2987f9f62897f33c42b5b29a40527e1d96129bd452cd7812e6838e6855533441e68962f6b7cb35078eb82bfa1cc0bf02bbd95792ab7bc70606584625561f45b1bda3246f6480350df97aa33776be740e31b66aff00a3c39dd14f4896f1c31b813d27779b4a0cbb2e0bfe7c755e374218f38f5768e2c3765b6658975bc7fb4885f4608a62f5d18ce74980aa9992d339cc23f69fe104c9b1d24aafb79453c446420948e1738b53adce737a5cc97f0272b9e5802789d74c3f6a9fe54a0c417efbeadb3f16881d8c6f0020c686a1e83d7136bbdfb0f6c14eecbdf6b6fa29f2eb87f25ca1cc17007a0ffcf98485f2c5b87ad076a67920d7ae0d31dcd158d678a9c1e91b8f9c8e5c6bacc0fbda78c7118fe74c86427865b421b6c718f7ae728f3f6a827ac69c3166424e1bb22b98bbadc13dc8aa39d8d06c2a33451bda3ed6162f2cad48687ef3b9ef3acc97b1d347569f99c39972dde789cc9d412574a3cf7d6180d10629c66af5d7127559c7700c01bae555c80231f52814b02c80f1cdf10f79ca51419bd190bbc3aed3ce066825981e594bd808b8320a77e868a71d3bca775934660c720b76ca65a1d60194f960e6c21f5c7178f8975e9a3be24e61267858122d3859654c5ff4bfe14817be8b727225cdc50e828a2abdbb93b8bd4ab26f5fe12e80932f5236aaeb5e4490f6acf124c8786162d5893253af46b9f5592a2f4632a3308cd3f395fe7f374aadfe96475bdf1a6e6bd8fb1f43d4bad952f34ba467dd28d7b38bcbc7583c20154c727d1e2de1f0f5a17ee8d296b85bccd49e8bf8a892c30dfd395d08f6b7fe1b2e6d035bbfef6e048bddaaae274d99661c7fb6cc33534c0b4f7c5860626de9ac2d405872281a8d1731a36e8d6f048abd616f40779adb0a15292efbca4673208bb4b52abae2c37c8af0f5c4ad0786bec6244514337affc5c0f8f5f2c73efde3159e6b10e41d2c2981423fe61f2621f5c97ef523339a5faa17537aeb62278c7a58b56cf3c944a8dbcab3d323260b5408c3539768ad6db12f9983cfca32f70b2858d237fb0efce21b8f467a821d218230a06b2df90d805023a626e128f0f928d721fcb2e3569a22d152b7dd3a7cd21697a7e9bd4c6194cf73ed9a3badcb12f")
	require.NoError(t, err)

	s := &Server{
		validatorService: &client.ValidatorService{},
	}

	req := &qrlpbservice.SetFeeRecipientByPubkeyRequest{
		Pubkey: byteval,
	}

	_, err = s.SetFeeRecipientByPubkey(ctx, req)
	require.ErrorContains(t, "Fee recipient is not a valid QRL address", err)
}

func TestServer_DeleteFeeRecipientByPubkey(t *testing.T) {
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})
	byteval, err := hexutil.Decode("0xf6c41c2d6baa52105e65c3c128fa7b3cbb67b2c5a30af6f984fdb144739b534585fbc683a6800dbfe777dd4ae8af54bbcfa894e56513279cad5b978d6faf0a6d9ea2646ecdf1337223210803b948d83ef3a45aeb5b7f9c88bc43dc0655d878dbf578e9d4a2eb83ca3d300d30ce21c6756f5a15d589592b61f295669ea8e55c95a5c494517b9f27c335fd760ab6358034bce0089ed377c13314eac650b9e70d24e165f05acc9f7cd51dad6c216ea5b0bbabb5941fc17d59a1b9ad4d6cdf01768aa4513cf3444ad7a23823e892be4c85e8024706a03e0878fc005c52ed08ef41e56192459093ac82ebf9189e8199997977a336c4fc97da1ecdc6e76da090f35e93aee367c1457998963266e59338c2c241d113dbb5a48be82c9e3a3ea7b90d84dbbc65b76d833a54e522cdc41db212f7aa2f741502ca9d567ab64574454206c33a72effb676007a4dfdbcd358865df822d52fe2e74684122dfa0a8b7382b69256543408ea42785cd4f7d93d26fd7df702680bba33558170549c162d1bf69493e33970aa33b741a553902dda80a76bac4e38bf849b30ab436f6be48018889ad7be38ae4907c52e81f94a129a841d7f82d2dd2058d4f9c6cf0c914f245445397da1e6dbedf110e0bc5cbfa71cb7d8abcd542968fee270bb3be39f6aa6fead4d35ac63213be8f194902d5b6a85f1c29d7a16fdb9a2d6675048d7672d45021eaca3a742672524bd77937f5e4881abff9046375cc6245abb688a6387a515661a3c5d077c630b5883d9fb0f50e804a8ff368413db048e1bddafd563841d113ad54c12fbfe02b6f8901fef312e0fa921a435c103338582fa310c3cbe5e26319e04d1493c46d6216f5b7455d9567640739d4abd9d3897634faa612b0f2054a5da2379ab374da50f406bfd82762f9191e90173d346af7ec6b43026b556edd4e7c3d5e85ea089961bba7a1e441e21a96c6b34a74bca18788376ac3e27cd24443716d98a9953c24e4f60aa36846cc1a06dfbfa5e7eb6c389a6a687dc2919b6944320a7a7960274db334bc2263a8e46fe78eb9f3d6f59fd1e908d74e28283bafea01f528bf1b475f23cd75725de3e34a3354abd6f4896787b7238673acf362969548dc5ef8de5e8aecf20b29ae329935536df1c547e8047f94f12d05142ed1b27a331d101b307ec61d34fc226c3fee2aef9e9a3beb99c37d8ce186579a27301fdec54a620ab037ddb28d316c8467120203977e24b6fa801d8524910e0a849da79efd54600da1ebfd2beb3ce35ca5161ace13fef5bdfe8a5a415e0943b4ed27fb2f6182aea57871b9a43d6d7e48e1c667db6c2b5ba58348b6e5ace49bd2a6a662813eec2aeb40feaf6a2f513262adfcc226c0d853be7cbd981a925dd77f3aa42bf055b550470c94ef1d1e4cf2f3e0c89f3ba74ed4ef2e49478158873c71e4993b31ab73c16b084a66abe789c9b5059cfae7f41a3b3c2ad0925976d342ef3703f0ef37bb8acbdaa4753d921b6c94e5206b981a33e0ffa42e8edc578ed980fbaa0b12c75a149c4ab1dfa9d1c84a99a9b9bcc92fe2cb7bfea289d21068719cdaeddfed24f472af45110999fec25e20ebd5912f9a46a86c476c4218a264c5c600280d1f5ffb6422c9e052f56b18f2c072b0e32a1e1c5280c7fb387e2b869da701e73359907a5ff402e3f8a0e0d3689726a96f4e17e89ea2697e08a124fcb04ab235c954dd3a971a7bfa55a454e30ba1b650488d7c38fdc594d7e3421703dff2b9c4f89839b786c54a7a771afbc7d2c08f3dd780bb144aa7ed6db5834e818f1d05544e3c593c10b918d1f6a2819e142edac44d5a7c0124a7db677281e71a88c8da8c56aff7a38ec6cc85fb1a394fe7cc047a67cd40b1831bdede39b47269916dd7d2ae0f44ce18ebc5be2704a048bd39456810f38970634179ba8b7cb791e43c0ce55569180f1405272af483219c90bd55451353e87bea6f0b3268a7c3a0db2e7fc7ef09ee39c758b08a200aa9f7826ded60f4087ec4f008bdb9ebe071ca1929b49a756b369fe2e33a9b478aec17d0f6b5f3d53f550434e5ffdc1f42362865871d53e1ad10a9566804f56b7fb6d31d11b748cb18a014d05361a0eac3da43108904aaae0effe61ce65d999587f3514f609c8e9dd2c334eea7514fbb3877113e1802c1d5b1ea628e6a3c74a34f0e280e8f3b0cb878c6a14519b7c6ec4fa5c6f181e93b90a3bb937372c7bae60f87f4cdca74ff455b7aad4c347606b8c92662d00665ff766eafc95018c79d78f3198333323b507f9cb2cd661e3d01201c213e85a461f3ab02c7abd4524fc3762912c483f7cf6d4fae58720af67ad3ec70112f90d02e2d4170a582ec5f1015b89b8eaebab00321efe4e1d2e72aa43212c7720f6369a9e557aafadc77686f5ba77321220acc84e44aabb32bb56a821a34264fd44223a9cbbf04a37a270d4910de736da028d3e543adb12ae1860e3efb281d6763607f919e82a135d5cdb2976ef6993c22c842932478fe3e1101794be57e55af788e5a994c65f5e17a8d3df930a05dfd9344927b6808742751e14c5ac1fdf31573678134740d43c49904d18a501d14105846802dca4333225ff39ab76238e4239bb0bb82e54ae7bd5b9a3b148abdb915953dc9d5eb2bf9a3fc401d4adc7c96f4fb12c5c9f691ba67ebbb82311e6c1a53904361e1220deb23ac3b474c35994a545a55b7cddab72d6b15b28d2c6a19c65e1d11eb465ea69533a82d25e5f1d59d87f80eecd7e2d27cc6020cd17c460bf2a3865530f892a7713f7345b6585c4a99b833b86440fb81e2649429b10e2d5a5f868043d794b75eea4ec4cb4dfe35b54659271f12a00877dc394d33e76c9262d98d8a4a0b24c0b24eeba8a7c3982ae1b946acf4d332c414c7d99e375287f5529b2d03921da76ec37d42f7622246d8af8d5d8227124c01c62beb79b96c0c0b13a34ad07fe256b234cf3859843a7fcb81360fac6d196fbd044b4627027412c7779a26e4fc5ed655428b3bbd5fc04960638db48c539205d6804b43ac9909fe4706f0a9b4d90d5087f899b15bbba8e3560573ee2ff8cfd103022f6b26ead55d98b5e2b58ec884d1b724f7bbd5254eb6740009e4541663a29f062b1e7281a5bca4d5456fb8feedb5d24df137a6c62ce4b2add9e2bb682acca91d403b477bfb61f28525840d0b37367573391efa6d003286cd4fda89e17e109005b217e2e63b1c7d581480e779941ea3facc87b1995ee177b62530d480723fab9a233404855f55295c461ee471d4467009e7e768271f498c16826a316c7fbedbf1c55c95f1e8ffc154e58c02d826080bfd71c8bcd95b160b378af4f6e8ffc246a1a7fbae3508aeddac603e1ec1960db30fe0fe751c9c72f33e9a1abfff9a78b545e88603de5aed94ab3d86417d3561e5f48debefdd5c94b0760eb3d106969c6836e8b56853cf34a909c9e9e099c48b149df2381667a7ef103292bd2a79fe543f6706e563e858e84ae741381cc89fbb6ce40d9f2838739c53fdcdcb783736ebccdfcdacf1049315eb3b57d96d39a0361a69e4c20d493dbb4ad4468ccaae0c0674ecef4e9bdef2d9f2aa04e1938bd1ac1f07bb9acd71cd811874242a19cd86d52a70aa1cd190ae2800127e545a7230c8f0bdcf5f0b7d627304fbcd85d47e995a992fd6134377b975d9")
	require.NoError(t, err)
	recipient0, err := common.NewAddressFromString("Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000055fb65722e7b2455012bfebf6177f1d2e9738d5")
	require.NoError(t, err)
	recipient1, err := common.NewAddressFromString("Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046fb65722e7b2455012bfebf6177f1d2e9738d9")
	require.NoError(t, err)
	type want struct {
		QRLAddress string
	}
	tests := []struct {
		name             string
		proposerSettings *validatorserviceconfig.ProposerSettings
		want             *want
		wantErr          bool
	}{
		{
			name: "Happy Path Test",
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(byteval): {
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: recipient0,
						},
					},
				},
				DefaultConfig: &validatorserviceconfig.ProposerOption{
					FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
						FeeRecipient: recipient1,
					},
				},
			},
			want: &want{
				QRLAddress: recipient1.Hex(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mock.MockValidator{}
			err := m.SetProposerSettings(ctx, tt.proposerSettings)
			require.NoError(t, err)
			validatorDB := dbtest.SetupDB(t, [][field_params.MLDSA87PubkeyLength]byte{})
			vs, err := client.NewValidatorService(ctx, &client.Config{
				Validator: m,
				ValDB:     validatorDB,
			})
			require.NoError(t, err)
			s := &Server{
				validatorService: vs,
				valDB:            validatorDB,
			}
			_, err = s.DeleteFeeRecipientByPubkey(ctx, &qrlpbservice.PubkeyRequest{Pubkey: byteval})
			require.NoError(t, err)

			assert.Equal(t, true, s.validatorService.ProposerSettings().ProposeConfig[bytesutil.ToBytes2592(byteval)].FeeRecipientConfig == nil)
		})
	}
}

func TestServer_DeleteFeeRecipientByPubkey_ValidatorServiceNil(t *testing.T) {
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})

	s := &Server{}

	_, err := s.DeleteFeeRecipientByPubkey(ctx, nil)
	require.ErrorContains(t, "Validator service not ready", err)
}

func TestServer_DeleteFeeRecipientByPubkey_InvalidPubKey(t *testing.T) {
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})
	s := &Server{
		validatorService: &client.ValidatorService{},
	}

	req := &qrlpbservice.PubkeyRequest{
		Pubkey: []byte{},
	}

	_, err := s.DeleteFeeRecipientByPubkey(ctx, req)
	require.ErrorContains(t, "not a valid ml-dsa-87 public key", err)
}

func TestServer_GetGasLimit(t *testing.T) {
	ctx := context.Background()
	byteval, err := hexutil.Decode("0xaf2e7ba294e03438ea819bd4033c6c1bf6b04320ee2075b77273c08d02f8a61bcc303c2c06bd3713cb442072ae591493")
	byteval2, err2 := hexutil.Decode("0x1234567878903438ea819bd4033c6c1bf6b04320ee2075b77273c08d02f8a61bcc303c2c06bd3713cb442072ae591493")
	require.NoError(t, err)
	require.NoError(t, err2)

	tests := []struct {
		name   string
		args   *validatorserviceconfig.ProposerSettings
		pubkey [field_params.MLDSA87PubkeyLength]byte
		want   uint64
	}{
		{
			name: "ProposerSetting for specific pubkey exists",
			args: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(byteval): {
						BuilderConfig: &validatorserviceconfig.BuilderConfig{GasLimit: 123456789},
					},
				},
				DefaultConfig: &validatorserviceconfig.ProposerOption{
					BuilderConfig: &validatorserviceconfig.BuilderConfig{GasLimit: 987654321},
				},
			},
			pubkey: bytesutil.ToBytes2592(byteval),
			want:   123456789,
		},
		{
			name: "ProposerSetting for specific pubkey does not exist",
			args: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(byteval): {
						BuilderConfig: &validatorserviceconfig.BuilderConfig{GasLimit: 123456789},
					},
				},
				DefaultConfig: &validatorserviceconfig.ProposerOption{
					BuilderConfig: &validatorserviceconfig.BuilderConfig{GasLimit: 987654321},
				},
			},
			// no settings for the following validator, so the gaslimit returned is the default value.
			pubkey: bytesutil.ToBytes2592(byteval2),
			want:   987654321,
		},
		{
			name:   "No proposerSetting at all",
			args:   nil,
			pubkey: bytesutil.ToBytes2592(byteval),
			want:   params.BeaconConfig().DefaultBuilderGasLimit,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mock.MockValidator{}
			err := m.SetProposerSettings(ctx, tt.args)
			require.NoError(t, err)
			vs, err := client.NewValidatorService(ctx, &client.Config{
				Validator: m,
			})
			require.NoError(t, err)
			s := &Server{
				validatorService: vs,
			}
			got, err := s.GetGasLimit(ctx, &qrlpbservice.PubkeyRequest{Pubkey: tt.pubkey[:]})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Data.GasLimit)
		})
	}
}

func TestServer_SetGasLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	beaconClient := validatormock.NewMockValidatorClient(ctrl)
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})

	pubkey1, err := hexutil.Decode("0x12f37c09addf59b5f44d76e44343361ef93cfd6557dfe7b599d48db70426b7592614dc8a700ab558a0425fd9e74ffa29e6567280e128cd2d75e7d587e089c0f6f2c8c0af4f27caf93168f48c6ada7c35c254d66dcf5056c5f34a5f69eaa8c178223fc87ea0c362a3f11ae9c9b6ffde1e55cb0ec4e27ec1c4cdbc980283e697f729ce329ca87ec1697fa77623d3f00abf62d927e4391d8bc9d5a49f94ab514c291b540ba7732f210cbd19048ce8c0077f8c23be34f14e74047f9ec06a87580fc71efa8642f3940c7016ff2e1e7df188dd74f3e082db46d9940f93b9b82c3b3b6011dbabbff2e7c15c4ce425b95c374f5fd21a5495816f9736aa601ed39351fc9043caeb109c4649713e1b64f26c9c5a19484c82db7fbc988edb02245a43b3228c04da0691e17a763c79fbc9d4c81d49d471067e9c8480d994a6ce8edfe367e84bc070492a999949f3788f25dcab3796ddb2803edc6ad330f90dbbc3f53c5aa43991303bc227cc2cec026dbf18a6e0168bf806a669ace35cf2a93633a797f642dda6e4a20702c1ba892a0bfef1e183e23759cd5ceae93d625eb0192edd72dca3f90bd62db4d40e8a6dd45d290d22509fc8d229466847be596d4e35fa31bc88362a16f1b5055501355ef7164e35a769cf6b3be98dece1f981707994f0666d1492c7c95345382ea83a928f13ab140f8ce31f349b59a5cfe778f63b20a0c87a880bac7f4a162df03bf974eb8d98f08e694df1b6b16248a67180dfc4d35425c1e42a07af1f2bea6b9235ad692051e90d864f4aaebf724deace2d103f3b4cfe9adf7928be71cdd9da4f4c4743b916e611376d9d3da84ccf5a6c8515780582de643388382b35278cea5ac5fe540cd7a0fde161c0193be7db5a5be86364cb0ca758caaf87943d054685a7cc34e0d0c459e2c35f2032cad55dd4114d50e18c1f2e0dbec69bd560955a5829c0c2580b6495add64dc76fe8720693dde42866daeb64a03972e4209d5071efe66930c9a35f18d7b45ae84fa56a011dee5f17dab4ea5c2c84b24abba1c1a5480ae1dbff984d3e7c8c338b68ff741d4930777247cf542ad0f2d65df788a243ea00f13e7af19d0f9b77a8b426759b2e380060145abc17bc921daff4c5452314f421be3a046b6cea02c456c7ea3a1e2bee868bdb1b4cd42dfb56e7ff46cfa797a1cb72959001a7bd09df533113d237f4b2e0729eeb33cc61257c5d2cb3f660610c7fc5a6540b6d544b16dcbf2ce028f196da87be0c42da0bbb74f63e1d02b9413be6c05d1a10b7d09ce1bf3240389fc72eb69d3f9f8272b18e4c973b9eb1585ac003f895b1bbb4055c8890befd3b5160c4b49fa6034eeac8c7c04ef9069a1efbe831add058aa1d9a777d98bbc5e24328a2cdf222293c91f791472fa465f2beb4f3741bb438bd16c913845eb0903fc9cd00a516bfb9e472b91642d9841b155d764f5288b25840d7fb9180702cfb47e3bd7276378d703a38af50c6d3da5d9f87aad8c865c493af725cecad426281204d33c69f953c47ba845d9b6f0fb9d0bbbf39f6f51ac03f9b8ac093029d914e4159f0d56bfd89a461200fb0cde02002917a78e93d6a272a2bae8fbbc49ba3aa3610cecbba0b793d8fe057ad97643c665b7eba0422903c67652774b3c386253191e806d43aea04ccf8b6f038332be81489df46138783fd613f60a121cdb32a57757afc98f6783f2b42e9f245322f1052842f9ad788783345c128ac11c0738be9d828df5f4ee2f5e716a74814d49fb09be70ce49fed21894db7662499ac74a6c2fb535ed07b40dfcecde2078cc8f4056746c89af3c6647b3b2a956237daefe49927eb8969d302d468c1c1d070b0d2872cb29bd4d3755dbdbdff67a07460f67e599bd3bb76cb109112fdcfd1cee3a118f6288a32112326beba7167c2eba60b613d90ff6128488ef4b6c1fd772336fdcda86ff414604060decae0f211afbb4e3ff499f3aa501da9b355e8adab8e4b914c7648a2de48a4a06d13b5c1b71778c40baa7428e43d373e937f5782aad8bc3653d7bf789101a89a694ef45ae5a362a91080bff8561b6cd4d88774ec673ea5877285b5857c4569d248401527e04d1652cd8ff5c1dad7adf4504646b63bd00293b1777bb4aeb8716639dfd9604f67caf6e47f9495420802c36a6932f523b50f6fa81c719bc12712f29cfc202cd65f8e16f7853c5fecd7837813df963f8001c4be9b62e27b3af9354c40e8675b6eadaf76f8bd35231aa4e0865133330f6bb639ab89ab926b1b045c70370ea7d39d54baf669ac78df80b55ec10ddf23be7d3852c81a2ebae9d3317f52588480898262ce5e5a7577eda0be12485131a3ac8a8914d580d0800390ea3301bcf1f90b8503122ba695858d8e9f87f57d9c9633827936e0a16ff5d6c5fbac06fab9def8d03c743e39c93d146861637cc58f9437381284030dba7daf4d77dbadbe8584242fa4308e38706e95041fef67e10a1ea1dff8a3268cab3ffbe2a77c0bc1c949aa24cfccae7f1431a5e43a2a7bb3c4e09a2ac87c49848d38e41337568ddfcc2ddd7c631d0486664e0fe5f4fcfe391d2dd33e0546ada1c74dad4a7cd41d1137d47cba2da38719653a6446f0afd4b121380dd316cda0d278064b9d3e5cdca0b2fb9f4cfb9c41dec4a39f0b5a178628c18f7acc92786ca093d64a81fa2af501a84908344acf3490a9146139738bb6bfabd76a06eb531d1756d321b8358544cca9f795615c4db5140208a3e47555b41594acef2737c33dcdde9c421662e1a8e120edb62eb430d1a9526e0dd2bf7a1ac5eb5fedc91f9314a8b64c8b69e2e6dbc9c20cc14192a33a751290f1f935913b2dec2ec13cdbec7f8e47ea3fcfd78b54777304f9386a8d79500399a791e1c62340a19b768f5cf1dfb902c52cf3fd393c1f24b107549d8c690a0e6f7d938c3a043bf9e5d8d4c445a7e1475c45ab9e05283b6a4d4b9bda84ee904b7ef29345fa0c9d630c0315fc6a7d21771aff564973a7082326f48e141774a584f84df052fc85fd4ecb2868271ee1db25e89bf2db5991b0c188f442b9fc31d2383f465c177e2d2b2c14ecb29ea44ad93898d85a3549ec497ea64fd15a91bbffea27d1155850b057c0c96c012f9db6d48deb3d56126973b4ebc30f16b8a3e3da942afb0a1b21629e3e567e5e52626b615e5d821656dc633ad6cf146f006bbe0cf361b056061b14c22b847b5fd6dac319af4a3cd2291d36a0ac5c4105fd4b4b151f0a93e615993f5ecf0e024c140ac4b397225646e8781b4ea2af5e695cb86ef7ae80f77798bf016c2490233bd41b8245978dc952d684525e5f69e4cb33570623f3e0e36cf31cbc9a1968db12d8bec24470fcb990ac3a298b2bb40a2a85088fc130b230abe51a12101bf9d8cc904d5983f67d91d981c5fa92b6b3d2517f471654dd4df20f9971be3d6373f474647832ff7ecf54f7a45303b137845c032970b1469cdc335d440455b15a54722716f9427abb52dee48f6d04564a3e3e840e3fe19d5319eae39c57d3123f47555e8c9243d9c8c18a44879fd3242734f5f30ff8f7ff47ed648bff41be0b60044ed5a36039dbd5ab1f3c13d62d73f83815b5f436b3bf1cced4f1609a6fba1bfb6a4b267fd13af494dd08ba05e10a805b5b2f001912e0a01bb5801b40a2999b93dccc7aea6147")
	pubkey2, err2 := hexutil.Decode("0x02654e644cd0d49582e40d6b6845e6aa752e98056e5730c79468fa6757e01af72d5ac1b1fe511c8b62eaca54ecef3451997b879705be3cbb5933431679cbfe460e9807f4c1e8acab43e240b352e15f9e9cbc1095ef6f7fe8f74aee8659abfdcfd29ecb667eeab9e5a86c08c0261508ec5dc36462079d07b514b26d15276a7d405485c517c64a43c393ae5a7783670af73b46cef472eb4d1a9069e698460f1c2df29c67f685a1ee0bdbeb4cb38ada1bdfca512bce3d11cd3fd7528948d011db59f749595badd69bbf32ad40753f30bc8103368e179c6b1efe5f2ce65d79e2145be6f3c49b9efbc7e68e6675b9c485c528e407b9a237baff8331e5648d6e66c9129532f5ec4b0c9ab5c831d523bbc02bd2b00ecf4f8a92802185b8144a208cb9815cc527dfacebb26659e7dd86db9d7fdd84fa6e8cb97f1b0c5969aaa9161969b4ef3b0d4cfe1611b9747974dae01f03f23e46ac0b7aded9291a9886974e840af5a0703357447db9b0a44c358484620f2635bcbb4b42b418b5a572b898df1e6c37a693cbf363397678e7e3b8bba6a13b0e392bd4d617fc764f891428f4a5f3f73c3cdc5a925d1bd982b295020b08e1c7e3a5e4b68370e6b8f3271f5b514e2ba9371d8a1421bbacc7d8750574700636184715c2c40a12bfd45b596b3104a52405bd22303a3f4fee8f3863492536c84079d6e4ada9f17307873c409890fe46ad6d74209d93bd6dad50e8afdba72863d2874b0bdd5ce11e53a52cd39446c8a86b20fcb7a9c953408297781c80f460cd640dd728db817ce625b872580d11342bc7c33422af7ea1f604ec9685136c7413fac82b66fb8b9c8b9f220e0088723444d24265d31ff8f2e3fef5291f1a13ab288e932da5a4d2f51b2fe4dd0b6fad9893e3afe105597bd5779da8735953eb824737c46c37daf9264be4068591bc31d9879a702d4cb79a42eb27c643dd7c5a1cc11af5068e19206b5f139ab9cc625b11bc3db3c46fbb5f653284323f45aae5c6988c38827fcf2cf893ddc6bba4f8a1069ddc860405c9faa3aec04fc8e4f9386cc00e11740894b214fbfc806b7c346f34fd8f1b0273ec2a827d534269d1368e5b82200576b6f61c7044c50c3138a26071f86c568b2d79f47e358f31bd93f4310ba8a7525988e78f8b5993309ea86c942740a4e4fda3beaef0a8731e67a0d01e9845c0c7dce26273fdce538505c6335dff7beef810cf43d6e26e182e92ee90d3ac2e3837022761f6777e4cedb05a8f15511f7f0402e678e08ec457e979b58230a2c4f483846a3f3c2a8d538130b827c06eacac865cc84264b29328c3dade7254c8395d31d3fb22696bd7780b85c541e8b0812e67225e9949127f3ee5a1fda79581565e84675df03b04b7a41bce9760d8b1345f2f0ae750ffe090173dc232accb302766f9a682bbb7ab677d93affa8ab8f992a2b5760c525840493fb4961aac8ac7d3245305d5989614d8303ec0b2f8ac712ddb6be355cd5fc88e40bd392cf8c80d8690fc5cba52929692083e1a1f48f93d0ca221382a4cb3f15ff6b740dec50491f55574da1d6a26de7b5074a5b3b03f2782686bf6751b490708e9851cf75ae41da6633d84d1a834ba114778f5f9a8975195f590d5d4504d577111b364b2619c41c0d486e4f4cc4db5a1fc3bfb5f1ae60cfc4da31f77ed06496f1465ad9a48c3556451d3b652f65d86247ad88d96261d38aded9ad09f1cae2d49d77f3e5c4246ff1645f6dd6bebbecd3ff480e8e47e068c9d032150c62fed1f22cb47e4ec934a53b2dc50f5ced91f53e7d7fd0e19e11c4f8244c59ec6fbc03fba6d477bf2d4f04e2c3a05bcd573ee4614b1f9cee7eae9bb3ee0e63ca6fa47ab8fc37a1967c6dfabcf21c8bbd22e435e17ef37102fb3932c955df05a899ccb3b18643894b2b30b9e902afc8e91fbe7930198c2cde89f109041d2e54a2bfb6ddb8733f01d3b6d3b7b3574ec28b6cb9a56ce93764ed848ca1d0ea0be5ca8334b5266b26be23016f19a3a825f6080a60a0223836f03d8b3c49a4ebcf264f0d1eaa949f4f56d6b1fc05700524e5cd52322213827545b0f73881d5b3d64f7bbb4d68fd76afb323f54e3c286ddead367d069f1a918cdf2a245d0389b9dfa8b401d181f105aed73dc9943b14c4e34478b66faa7b1b813228aee24d2c31c108aec212bbfdad179aeaa0d7fb44936b79c48097dd9e0d518473a0ec19c4355e82e51b4c5fe5fcb082e6b17bce2fe4cefaea4ea4c04f0e4a6520242a2c6ade53c58c1ef57982bf8765314ba2545d99e9dd715847b2c52dbd31a5c6a96c679214ff797b81813b6fd50eb70dfb85acff297fb66b3904c2d4be79dd41e7cfbbea29c2814dd03f6cad22e0f6da2c33ea14829d338ad387b7d7e4a10068f75698c8a97a3314f7214b09ebdc3224ca0ec00a54d3103547990a796a8e3ddf1df7e82bf1102123286267b2c68929a0e620152097d8eeebddba9b45cceb7c698baa3b0470eaabe540dbffd5ec43e5cc4cbeda3c161994ca565b28d6dbb23957b12ae36cf5bc87042cf651ba3b72dfd651919949b41f1452c8f68f3aadcfd52934dd8be2dcee73fb867fc945cd27dd2d2816a5f8d66f5691c87be71ebf7c10680322c5b713e41b13768a25e852e771f55d425cf4a2a8f96d288a2d55f406f218478ded49df23c154f9b8b8980b1a6c0cf7ee0b5472e494208418b390136e1e47f068b69e4da5f84fa6e7a59c61dd168cf2dc8741961979631e32ae25a18410ad97a3f4d4e04d9100ac1cbc9263e3031634da4a72eb1e1523020dec8029953a2d3b63648ebb89b1ac24b54f5c9a565b2b8da252d0a6a770b6582794eec5e17d6d3c44816776f8687487137566d029771bd63588d044bce6693cea673633424a6349c46e47cfcbc58d61198a5fd7520ebf252b90cd835cb7a9be372501140438d195c28e01b2412700fdf8dd53ca0013584f89a4c4b37fbc3625f5841f1818e933ce32f31523fc16b4448e0436a97725d5a520d8dd160445cdb35195c9ec3b356573f4a0069afc9b2cbb355f749b880c22a6a94adc9bcfc8937b6739d92017c057615210a7a8df27d0b17d48ae76f8306b28f047f4227e750380bd02c776aaa1e278285e298f0aacbc7aba7b2ce92b01d7d7e1c55531045b46dbb42d616b0889687661d21b7b7b2b4b095f803810ecf818f93e384e730bae98d4289ac5d246b11863b329337949a8334eeb54b6637632e175c135e0063edbb3f017b875e63f5eda199b97e09351afa325c8e4c4ba2641d7ad293dc8d0a864ff78789ad50ca9b9d6ecf532c0357c63f43f73fe63785a0518eab62d05e1c353b62bfd170daf1ef24706ac17f8422debdca51a5ea26f115997f032872dc766c23ad60dcdd36b8485eb3d323221adfdf8844f3eb67ab14fa5a60561dfa221311b94923d0c2d2a10f8961d8da1f51bed25ca2c68b6b82621ed9607bdab35f8e0c8dabaa3140b0344b190046651faf2c479a7f8ec1b9552749be9c73b68cfd3f54f9c5472191e44d1779aa5eb8d95210e8411816e4a1eea1bbb1813b1489045755dcce17add46c164ee1dd6947494eb40056f610a839412709943052c2f211981689a9cd2123c401b868ec14632be97515e3be0e1286c3ccd1ac683f4a6d15488e7033abc6f2d0ae089dad537f18525618")
	require.NoError(t, err)
	require.NoError(t, err2)

	type beaconResp struct {
		resp  *qrysmpb.FeeRecipientByPubKeyResponse
		error error
	}

	type want struct {
		pubkey   []byte
		gaslimit uint64
	}

	tests := []struct {
		name             string
		pubkey           []byte
		newGasLimit      uint64
		proposerSettings *validatorserviceconfig.ProposerSettings
		w                []*want
		beaconReturn     *beaconResp
		wantErr          string
	}{
		{
			name:             "ProposerSettings is nil",
			pubkey:           pubkey1,
			newGasLimit:      9999,
			proposerSettings: nil,
			wantErr:          "no proposer settings were found to update",
		},
		{
			name:        "ProposerSettings.ProposeConfig is nil AND ProposerSettings.DefaultConfig is nil",
			pubkey:      pubkey1,
			newGasLimit: 9999,
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: nil,
				DefaultConfig: nil,
			},
			wantErr: "gas limit changes only apply when builder is enabled",
		},
		{
			name:        "ProposerSettings.ProposeConfig is nil AND ProposerSettings.DefaultConfig.BuilderConfig is nil",
			pubkey:      pubkey1,
			newGasLimit: 9999,
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: nil,
				DefaultConfig: &validatorserviceconfig.ProposerOption{
					BuilderConfig: nil,
				},
			},
			wantErr: "gas limit changes only apply when builder is enabled",
		},
		{
			name:        "ProposerSettings.ProposeConfig is defined for pubkey, BuilderConfig is nil AND ProposerSettings.DefaultConfig is nil",
			pubkey:      pubkey1,
			newGasLimit: 9999,
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(pubkey1): {
						BuilderConfig: nil,
					},
				},
				DefaultConfig: nil,
			},
			wantErr: "gas limit changes only apply when builder is enabled",
		},
		{
			name:        "ProposerSettings.ProposeConfig is defined for pubkey, BuilderConfig is defined AND ProposerSettings.DefaultConfig is nil",
			pubkey:      pubkey1,
			newGasLimit: 9999,
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(pubkey1): {
						BuilderConfig: &validatorserviceconfig.BuilderConfig{},
					},
				},
				DefaultConfig: nil,
			},
			wantErr: "gas limit changes only apply when builder is enabled",
		},
		{
			name:        "ProposerSettings.ProposeConfig is NOT defined for pubkey, BuilderConfig is defined AND ProposerSettings.DefaultConfig is nil",
			pubkey:      pubkey2,
			newGasLimit: 9999,
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(pubkey2): {
						BuilderConfig: &validatorserviceconfig.BuilderConfig{
							Enabled:  true,
							GasLimit: 12345,
						},
					},
				},
				DefaultConfig: nil,
			},
			w: []*want{{
				pubkey2,
				9999,
			},
			},
		},
		{
			name:        "ProposerSettings.ProposeConfig is defined for pubkey, BuilderConfig is nil AND ProposerSettings.DefaultConfig.BuilderConfig is defined",
			pubkey:      pubkey1,
			newGasLimit: 9999,
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(pubkey2): {
						BuilderConfig: nil,
					},
				},
				DefaultConfig: &validatorserviceconfig.ProposerOption{
					BuilderConfig: &validatorserviceconfig.BuilderConfig{
						Enabled: true,
					},
				},
			},
			w: []*want{{
				pubkey1,
				9999,
			},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mock.MockValidator{}
			err := m.SetProposerSettings(ctx, tt.proposerSettings)
			require.NoError(t, err)
			validatorDB := dbtest.SetupDB(t, [][field_params.MLDSA87PubkeyLength]byte{})
			vs, err := client.NewValidatorService(ctx, &client.Config{
				Validator: m,
				ValDB:     validatorDB,
			})
			require.NoError(t, err)

			s := &Server{
				validatorService:          vs,
				beaconNodeValidatorClient: beaconClient,
				valDB:                     validatorDB,
			}

			if tt.beaconReturn != nil {
				beaconClient.EXPECT().GetFeeRecipientByPubKey(
					gomock.Any(),
					gomock.Any(),
				).Return(tt.beaconReturn.resp, tt.beaconReturn.error)
			}

			_, err = s.SetGasLimit(ctx, &qrlpbservice.SetGasLimitRequest{Pubkey: tt.pubkey, GasLimit: tt.newGasLimit})
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, err)
			} else {
				require.NoError(t, err)
				for _, w := range tt.w {
					assert.Equal(t, w.gaslimit, uint64(s.validatorService.ProposerSettings().ProposeConfig[bytesutil.ToBytes2592(w.pubkey)].BuilderConfig.GasLimit))
				}
			}
		})
	}
}

func TestServer_SetGasLimit_ValidatorServiceNil(t *testing.T) {
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})

	s := &Server{}

	_, err := s.SetGasLimit(ctx, nil)
	require.ErrorContains(t, "Validator service not ready", err)
}

func TestServer_SetGasLimit_InvalidPubKey(t *testing.T) {
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})
	s := &Server{
		validatorService: &client.ValidatorService{},
	}

	req := &qrlpbservice.SetGasLimitRequest{
		Pubkey: []byte{},
	}

	_, err := s.SetGasLimit(ctx, req)
	require.ErrorContains(t, "not a valid ml-dsa-87 public key", err)
}

func TestServer_DeleteGasLimit(t *testing.T) {
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})
	pubkey1, err := hexutil.Decode("0xcbb1bb9817bad0c730031d9cc48527fbf5b529b97fc40b88c0e3d7765e2eaba1b2892b2ee14b085d4f3a241610dd6b5e60638ef996159b1e35d57284240e32b83b2a82a437114430cdd54e006c0edebbf38e7cc4bf6e3abbf4a80e38b0d2153c92d8817e36da130bd969667b0d28c3713dcdfb8a100f8d0b7ac83854b65c6adbac3f9a3838dba0f7fc842814994a0a89da64dc207b8c3063dfe2dac8df5b23d723b315199579f34120aa819c3d825e578c2f9986298d529dbf87119a960e0e3d2eb8a4d235b2338fccb4b6b599c75c4bea5a2f958b8a3704aa809484d6f63aa861d4a8b5a7e3dae82c9f2b8b694b501fc028e346bdf0d095ee12676ce454fbe38b5d2184d5e75bd6900f105eae41e915293beb3068d2fed27a98b4191877eed8ac98a6ec316cb2fef3a41750e311ba8cf12e0630fa2f7ec9791f63c8a38bdf56cc706e2b17199f43791c0b80f912a883b33eac3c1b9f4f42214c89cbf036d481932d2c66fab59fd69e2073feb7995c72f05f6d13b9acf0e88632afe4970d513fcb5045b112b6522a89349e35544751aef1fd084d9af64d857eae90fc28ec3608721068856d0dc3b089c4715a25cbc2df234d3511c26fa198f5e55fcf07d39a95ea37b3e922caac23cfeb397c9b8e090f4ad3951f34df4b0033bcc64335e654fd757b42ef4bcf2c3de02c29ba34dde37b1f33ce181891649873347e8fde5dee17787a6d9ff21965cea700bc5a7c7a76d2f0f42fe461559594a5a64a4b976c50b26dc9d104510eea472fd9517d779f151a94c8d4a82063e05fe7b0f9da261d4cf473996da5e1728d35ff0c013307c63736b73e07aeea6630db81191a9010c942a0af12fa8bef3f79950aa50d020665fc1462325742088f91f8d72978fe8f0cc3020a792501322274375b81d5c4f107370af3742ee37ff2a0c4c29d600b21cffd2bda04a18fedcabbfd9839cb61d4b6275862f9a845e4083b0f3cbe08ec30cc8ac4c4ffa80ce04774ecf6e1ba5d567b0b10978f72b2327aabf496282a7cf82e2fdd4d55faae0c5f98ff8cc2a0c490e8bd4a3d0cd8f2a1806cb2d1200c8cc2913d9a23ef0102c4e811b1882b739ea2a276d6d317a114462d076dbd733f41d28c45a3b212cf1d8f33be6789d55c852f3e67dca55939f69376924e545d00f70d113b05a559a3548e684d7caa1f0a76a54c9fe593d2eefeb6cb6b7aaa65475b32ef416b7bdff594ab4c2656cce8a2f350eb417f73ef181aec27e28d8153d5294fece8c92279dfd66a0db2227cc007e9a85202401d7b785ced2d5841f54f43fc27cf8eb97d8262ef4979fb3ca8177534d229822a27f295ceef04705a15450c64e77c1ba57fe50141999e0de88fddd0f608676a17bc9cae7118e680664a2214da03987146ae339bd46123581be9575186f440d7bb2a9ae8f8e5910b56ac5f43cd92b26628a9f785a8e733249720eababb195ae07084e305800474f6f49fc003fdc4764031d0b29cc7828dd676b774f86b0f8c25cb9c77d2011f9e11aea3f357012b1b6309e2cbfdba825a3b4968871d94d9944cc711c5e5a4de8d120aec9f63a73b584baddc38fb5b8093049af701e36bf466122d7d08c21d645e9a6ec632343079848cff20892ef6e91741832aa54edbdbbf3021b140f414a512b5fc37eb056d87f9d5807ff4568995ce5da13d66f273c13fb695c4f4542523732fdcb4cf777e7c602b9e17d2a6338f89cebd1d9c87a99b9706922cf601b1d4829aee06f25603f510db7d764e4c868d4223e37e221665856511f7d49280ae67c213961d916b1f933a8a8a2f5b7b805a9142089be6aa58f628e1f508c8b928a90ddb09e859d6b0456cd371262fb97f5f766e3b1fe933e85edd72f6d495fa39632610376dcd0e618781470d3ed358a2b7e3b6da9c04546f54827af66ff948d4ffa91721ff8a472192bff812f8a912dfbefc1d9a6d8fc94fbdc329e1bbc06346cd8dd5bfed8ded98a5b677771528e1aae6075701d777cd0b668f46f87abce7b320fdada3ca113887de332ae0624b78e696f24f9a25807368e5b30ee61daeede55fe6c6b94803a3734079b84719a35bedfc190e028b2a40054f3f1a75120607a5d522610401ea3bccd3a86fdeae7178f9234ff5a815db46eb73e46e932506b026719941811c8826efb6ef334ad71a57e5141f2ae295fa064af890a7481621efa30cb6ccb261d71a51e5e0e286e14118598fe079f9a9ce0b941897f70fa086f53d7903532ff379b5caaae02f4a44bedf05117175225d319bfe109a3a301e10bad9529e619fe7436b8a81b610f8a3fae214695f4282e4f60adbc9686e8d158619257f7b7f6a4f15f0c582a866bf633be78e717a0d3c779d3bac63ce5ffb3d612b7742d71b97e1be75ede62e89b75b6cb4d98436622a492e40f6ad846aa157e8e425d50439d71329a20e7c5c7d2686d3c55d6aeb343a16beeb42bd0adef5df9e9c781818c23a670a565237febebdcc0d5247e2596df43b27154e30db4ea614843207a53bd1020b6a40be145bdf84d4935ceca95f4530a5854eb86b4fecb9e99da0c50f0b5c61a75e76602e46c63ec6dde848f7a04aa853598eed368ac267fb501c9f76868f37a4485255b210cadaf998556babfb4232ca35c0d040835160db83272f0b421f0c4daa0f1ac8e653b7d75e92b4a3230fd9d322e032be97167aef6f5ac06d89c1c99758683d86450710e1c6018ef8f0dae897898499bb5d78d80156e1be2a5d3a6d81e7348df95a9460be218b6d70ed3d2b5c93d5a9e9cd7d6f746b70c012a9546a567867a7447600b4b85c8de9434d24c87a534730ee9dbbf389511d9ca08d06f0d85622693f2e939a74cc9ea28dd6d8d69eca597835413c94579a365657080a98b28dd6d410984522e4a668b71eb71303b3bfc897ea253ffcd565235ace86628cca25b05671538bb185af6d84c91a28f7a7e8ee623acd08812f1d849809f6c0a7f5e4878bce62b4ac64531fe19095ffc0c6c412e18618e49e63f38cfe8e0c8b556e1382c27ecdd2155948323651a7c62fcdade4ee7a50d0c97a565e99fca329d54dc4d6763a8a049d75570aec5f863972f33f19adf20a3455d6a459432d049ef6125f0da299c5ae6788c92398dd4be82687495fbabdbac9312de90254f4cfc3a43b33137fc6b0ccdaeea17e2ce02fbf168602959ade65c22416a03848eabff0c21a6deb80b89b2e769d2437ed5687eead6355d441699c3b96cb9d828dc02214d6d588b3d31f7dd61fd64d6476d54d978e1282a1af470b6ec3abc04e46b262b7d497e0f097768066ca218437d06e8db8f200e3690b7b3ffa90ded251389eae591594b8af371322410e5a7ce363b3f529d530d9f40122444da7a7dd41769c0b83889c4fd4219dac258732628f86308b05f18f3637c8326c6c6a2097bd205f963c7784d10b4aacfec31596ff15e470cb2112675c544d2e7702922ba12cb59c6cad28b179f0ea5ad19a50b01a1634a4dfda8026881571972b040d41492272c4836f52edbf2732aa6f136b4c78a26abcb6a4d1878f09ab4bbe269d8757459918a2738edf154d9dd4d5fcf806778a5b1afa058aa7decbfbad9a059d0882ad3d1f0845fffa53247731f7dc34524b9eba13e0a1ae76ee7c001c8a7edec1d2ea4e7f3b52aaede28b918a73ea0b")
	pubkey2, err2 := hexutil.Decode("0x8fc209891a60b6a94dfa72c963bf3732c3925491b62abc3c1c3d59e15a1809be1f9696b07d5cd1cb25bf7ec0b696cfd809dcb444674651e15047b6364afddbf7b745ea4afc5e5eafcdcee4a785ebe5672fd66c0c146552216d3533b99610905d534a287afdfe61dbbbc8425a37523ab800dae0f28da25f8e1fdf5025b7223aea82d0f4c7350b4c7173a11c272e230bf67800116420e32ba81ab6ee78799e08ec5082b396d9d5007cbae6cdbcb88c664585424d04c71f8643c073cf08e2e2241a7c070206de9dd785f10fc80ba9447c2eace78ed4be3209f057caa396ad4de1be7b7da567ed2f9923abdd9c07c616aa966918d67f898525c368d9c50de29996f5f90fed4efac9f9dd0cb44d875a1a85dee4e8feec269de76492c89128bb5ccd9197cc8b879cd120c56a3c5b236c9cf4566478532e5cf14048d982558453526017a8fc2aab80d18ce4df4952278fcc1f3acb1d4ce2eb519490ba9f4676a2296fbde9be62331e9f558d9cc0302e4232f641b87efdb635885612cbc7ab33423c8d0f9396fbf71d4676ca7e2d3201736040dee85e6392f09d887f77fc92e88a048f8408b5b439bad401a76dadacf6b74d713307041e27f1bc37fe37b71cd24fcb8955b3515bb5bd4ecadcac0e537865a64f27a5cc06f7a977ebc782358bed93a28ae118d190af4dde6bc22f079e1d4ed9011216728118dd3880b213fc5148a90a3814355c1bd247174e54cae57fe1a35987e0f5c89eaa9f6f3a2cc16e859efc7472c6c2a40d7618528c1e41352585f40376e2eb73720f535b0bf0bada71c28c3199098543eebe16860b7ad6be701943e17ff50f02f7e1e9e7a349416a2426a0fc4b437ff70af2b0a81c9eea17e1e37d87ba48b5daefd040dbfaef83a445e2f08db0632cc194ee939c8c2d4bfcfc40012176417297fb35a0c3802408f8dc1b5697f9ab89c61d89213f4e138d862897652119c88b3d5d1854e6252169b43ec036cb9da8c5f59bdf5fb05a8310cf5cd4caad0b6c145bf862dbb56b7981c2e6665603e71d23c877ee081e69736399be345067eb40ea7731652a9474b16d9adbcdeb8fe8292d1fadf7787b2d864f6fee01960df3d36e9558bc7dab122b8ae47d4bd83c83336c1eca9c30b45d8cdaa0854bb75a62d05137551bdada85b47ed205bb7641288a38e8b31bbfe655348b5e1a33c627bba488a5751d2966082f641b1a7b486209de813c8dfed007fdc52632aed5018a79c5325564ecc21c56dcfd5d152e71913daffae66c4debf1ae0d6923fc2a568c66de823187c104269124c4913b2d7dedb5b4971598856e8c7eceded5ccce95086df8ae437e65b8f1252a6945071a51bd0539207c3be613db37fb5f8c5b1ae57783b6b9c8b3a43bfb29de34704a583cea6780a224292c805ee8ab849d05846b4a8a66688bfb60d388748ca9acbdc859c4c152aecc994ecf9d24bfa03872e67c787592b48ff338064ffe4ba5783a9db202120256876fc24023945cb9d01b390a2a5605d5c548b9ca6a168cab03eb4ebfc1ba8a16d955e6d5ff4b7d3887464ce8deafedeb951f6d0496b171b09a01eff825f531e5ba85a8e6a638f80e988a0bf7da8f151cede5d0923446d632bdcb507eb8cec875d88d508f450333bbb3f46bf55a8805342a1a48a38afc378ef53592392869d2b89cd7d2a10aa52382e514664bf72c52ecffe96599de0ff159badc3d428f7a8718cabdb0d58be818b7b1571e535b5d8a26c9f295db30224fa340abaf8ed24cd3a4918ae3697a4a3a60533b8a3b1f3b68d017c47d7b7ab64cfe62a30ba1b4dd0addd289e2342e8fd4f9d272efea92f5e483b83b8be273f9d843b2f92358bca73f1f54a9fbd9dac0d2b6b9dfa84bb6b78d1a311696200ba8753a071d5844cc324141876421a93e65b4eaea6d592f82c7193042af4c8ce59e8950e50cefdac07b7af669543504ba62fb56db7ae2d284cfe6ec721e358ec012679df9927ca583f066c5bd38fba8bf5fc631b9d4db04aea98f99c93855d9f8ccfa173ae97877f1f9a0a095f52d963c3f4d11fd97384f713b10ee7e411ef057832f2a1c0dc8dd9acac529e10a91867375136cd0b84f26b5115b6b71c48ef12abbee643370d0cedcb125613d15e941c8dba261d8b4d4b33913329c6c106a21f6653932d1ad452f9a5ee519638ae9a92090ee153330271b883000bd19d2662daa145640581aa5e3718091c82059bd39a592aa19a4e7f646e2fad56a5ff7d39ebb9055ec80e0d62377dcf9e215fc9833747ab729bb3867bb8cb626a728702555afa9d876313ac603a9635241a697307812e7e54176f064a76b109f24825a40f8c3329f953f3e222164f39bfd5f8b8b22c58946045d872a7db90eb35ca68478346bbef677481335942a29354a6a1809aa16b94bc6c50321f3bf85c2906ef0479ee6a670db8e3b68386d376bffdeee34cc5b4792c8245b22d1c9b28d21c62e7df518551ce78700fd68bbaf4776a83085549630e5457947e143e8aea94faf386ec6c285089e8c4838861ead9c765ed412c7c716feb067d278bb3e1cd4f7eac1caf65b04955c4389b8243ed1f0d18735dd76c0e13a2866e3440da64d079e627ff0ec46ad9e58fc78d7ad5d03341af8e0e7c2666cb9058d94428c5b3a0327c3031760c6d0f5c5b66458d5ef43c7daf371ed6c5dac2236b8c745b335c58166471a9dec35d0a6ac8da90e4c259d5ca2585b43f9fef26f68134c27814f01f7895f40b39ec5b27d193cf94f254817f83fdade3b90296d6facc89302e2ac83f749d54169035f23b3a34fef7dafea3b4d4978ceca0b065c7a520e0314b525b673d129d57b4e4472c56528406d78f06c5bb83cd877a63f3e75d1773de16a6b483f8bbf34c1a42ebcdad549b583d3038dbfdd87ddf17ab014e62bbe856bd93bf1ce9d1cd487d84aab54573a8ecf8ee1de7f3a1ee6a58fdff9a87ee26613870f6c531d86d214f0087250e20ff398ad733ef18dda6ba38a54f274e3b6a6bba92ca0b35819e34fa569c93167e8b67234ecaa9385b667ae08a24d572a57d8029dc245de333832d703cc53ae5e69fa5303b4aff1b45664ddd35a962c0c4b618d5dbee7cfbe8b8a9875a6105ada420344c5a968347cf72e3913b0cef962bc82d7dd1a27bcb207a53a033aafb7ac17ecbb28ade6ee23e14ff9a94c728daf511ce668ead02e75e51d9061c07d24326760a91c4ff8cacae22e5fa309e24f085ea3364e2c6da5d678a64ffbedde2182129469a81dc37d69e8c1505f62804d83134f8630b1ceaf5b537e46ed6ff39adcc50634dc4a64039b338dcc561282000713a50ded76bc432a8f16eb04d67ad00c22c1c831d33509e2fe0b791b32995a6c38229be0837300022b2ec43a680d36d2ffea83f8bd4b3b0299c0b337af44abc9c3fb632829d1788d66d8b0d680a0141ef804199a6575f4c1577b3768c5d488535441d0bbebb832eb49a59beeea373716e84c9d858e61734f6d81bec726eeb15dfdfeb0a748e1ba0acd6552c6f768813a2d9fd4f39405fa06c0b9c1862270d582612c1aa4e86e3d1e0e567c3429c9028ec2729613807f7f12153f36ac1cf578b5abad3943bfb2f17879936ae8fed07c02072515fde087e4f3f6b7922f729a6c9edb8eca50d4b70de056ed1d9060822161dcd43ebeab73e9")
	require.NoError(t, err)
	require.NoError(t, err2)

	// This test changes global default values, we do not want this to side-affect other
	// tests, so store the origin global default and then restore after tests are done.
	originBeaconChainGasLimit := params.BeaconConfig().DefaultBuilderGasLimit
	defer func() {
		params.BeaconConfig().DefaultBuilderGasLimit = originBeaconChainGasLimit
	}()

	globalDefaultGasLimit := validator.Uint64(0xbbdd)

	type want struct {
		pubkey   []byte
		gaslimit validator.Uint64
	}

	tests := []struct {
		name             string
		pubkey           []byte
		proposerSettings *validatorserviceconfig.ProposerSettings
		wantError        error
		w                []want
	}{
		{
			name:   "delete existing gas limit with default config",
			pubkey: pubkey1,
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(pubkey1): {
						BuilderConfig: &validatorserviceconfig.BuilderConfig{GasLimit: validator.Uint64(987654321)},
					},
					bytesutil.ToBytes2592(pubkey2): {
						BuilderConfig: &validatorserviceconfig.BuilderConfig{GasLimit: validator.Uint64(123456789)},
					},
				},
				DefaultConfig: &validatorserviceconfig.ProposerOption{
					BuilderConfig: &validatorserviceconfig.BuilderConfig{GasLimit: validator.Uint64(5555)},
				},
			},
			wantError: nil,
			w: []want{
				{
					pubkey: pubkey1,
					// After deletion, use DefaultConfig.BuilderConfig.GasLimit.
					gaslimit: validator.Uint64(5555),
				},
				{
					pubkey:   pubkey2,
					gaslimit: validator.Uint64(123456789),
				},
			},
		},
		{
			name:   "delete existing gas limit with no default config",
			pubkey: pubkey1,
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(pubkey1): {
						BuilderConfig: &validatorserviceconfig.BuilderConfig{GasLimit: validator.Uint64(987654321)},
					},
					bytesutil.ToBytes2592(pubkey2): {
						BuilderConfig: &validatorserviceconfig.BuilderConfig{GasLimit: validator.Uint64(123456789)},
					},
				},
			},
			wantError: nil,
			w: []want{
				{
					pubkey: pubkey1,
					// After deletion, use global default, because DefaultConfig is not set at all.
					gaslimit: globalDefaultGasLimit,
				},
				{
					pubkey:   pubkey2,
					gaslimit: validator.Uint64(123456789),
				},
			},
		},
		{
			name:   "delete nonexist gas limit",
			pubkey: pubkey2,
			proposerSettings: &validatorserviceconfig.ProposerSettings{
				ProposeConfig: map[[field_params.MLDSA87PubkeyLength]byte]*validatorserviceconfig.ProposerOption{
					bytesutil.ToBytes2592(pubkey1): {
						BuilderConfig: &validatorserviceconfig.BuilderConfig{GasLimit: validator.Uint64(987654321)},
					},
				},
			},
			wantError: fmt.Errorf("%s", codes.NotFound.String()),
			w: []want{
				// pubkey1's gaslimit is unaffected
				{
					pubkey:   pubkey1,
					gaslimit: validator.Uint64(987654321),
				},
			},
		},
		{
			name:      "delete nonexist gas limit 2",
			pubkey:    pubkey2,
			wantError: fmt.Errorf("%s", codes.NotFound.String()),
			w:         []want{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mock.MockValidator{}
			err := m.SetProposerSettings(ctx, tt.proposerSettings)
			require.NoError(t, err)
			validatorDB := dbtest.SetupDB(t, [][field_params.MLDSA87PubkeyLength]byte{})
			vs, err := client.NewValidatorService(ctx, &client.Config{
				Validator: m,
				ValDB:     validatorDB,
			})
			require.NoError(t, err)
			s := &Server{
				validatorService: vs,
				valDB:            validatorDB,
			}
			// Set up global default value for builder gas limit.
			params.BeaconConfig().DefaultBuilderGasLimit = uint64(globalDefaultGasLimit)
			_, err = s.DeleteGasLimit(ctx, &qrlpbservice.DeleteGasLimitRequest{Pubkey: tt.pubkey})
			if tt.wantError != nil {
				assert.ErrorContains(t, fmt.Sprintf("code = %s", tt.wantError.Error()), err)
			} else {
				require.NoError(t, err)
			}
			for _, w := range tt.w {
				assert.Equal(t, w.gaslimit, s.validatorService.ProposerSettings().ProposeConfig[bytesutil.ToBytes2592(w.pubkey)].BuilderConfig.GasLimit)
			}
		})
	}
}

func TestServer_SetVoluntaryExit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := grpc.NewContextWithServerTransportStream(context.Background(), &runtime.ServerTransportStream{})
	defaultWalletPath = setupWalletDir(t)
	opts := []accounts.Option{
		accounts.WithWalletDir(defaultWalletPath),
		// accounts.WithKeymanagerType(keymanager.Derived),
		accounts.WithKeymanagerType(keymanager.Local),
		accounts.WithWalletPassword(strongPass),
		// accounts.WithSkipMnemonicConfirm(true),
	}
	acc, err := accounts.NewCLIManager(opts...)
	require.NoError(t, err)
	w, err := acc.WalletCreate(ctx)
	require.NoError(t, err)
	km, err := w.InitializeKeymanager(ctx, iface.InitKeymanagerConfig{ListenForChanges: false})
	require.NoError(t, err)

	m := &mock.MockValidator{Km: km}
	vs, err := client.NewValidatorService(ctx, &client.Config{
		Validator: m,
	})
	require.NoError(t, err)

	// dr, ok := km.(*derived.Keymanager)
	dr, ok := km.(*local.Keymanager)
	require.Equal(t, true, ok)
	// err = dr.RecoverAccountsFromMnemonic(ctx, mocks.TestMnemonic, derived.DefaultMnemonicLanguage, "", 1)
	password := "test"
	encryptor := keystorev1.New()
	id, err := uuid.NewRandom()
	require.NoError(t, err)
	seed, err := hex.DecodeString(mocks.TestHexSeed)
	require.NoError(t, err)
	validatingKey, err := ml_dsa_87.SecretKeyFromSeed(seed)
	require.NoError(t, err)
	pubKey := validatingKey.PublicKey().Marshal()
	cryptoFields, err := encryptor.Encrypt(validatingKey.Marshal(), password)
	require.NoError(t, err)

	keystores := []*keymanager.Keystore{{
		Crypto:      cryptoFields,
		Pubkey:      fmt.Sprintf("%x", pubKey),
		ID:          id.String(),
		Version:     encryptor.Version(),
		Description: encryptor.Name(),
	}}
	passwords := []string{password}
	_, err = dr.ImportKeystores(ctx, keystores, passwords)
	require.NoError(t, err)
	pubKeys, err := dr.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)

	beaconClient := validatormock.NewMockValidatorClient(ctrl)
	mockNodeClient := validatormock.NewMockNodeClient(ctrl)
	// Any time in the past will suffice
	genesisTime := &timestamppb.Timestamp{
		Seconds: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
	}

	beaconClient.EXPECT().ValidatorIndex(gomock.Any(), &qrysmpb.ValidatorIndexRequest{PublicKey: pubKeys[0][:]}).
		Times(2).
		Return(&qrysmpb.ValidatorIndexResponse{Index: 2}, nil)

	beaconClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(2).
		Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	mockNodeClient.EXPECT().
		GetGenesis(gomock.Any(), gomock.Any()).
		Times(2).
		Return(&qrysmpb.Genesis{GenesisTime: genesisTime}, nil)

	s := &Server{
		validatorService:          vs,
		beaconNodeValidatorClient: beaconClient,
		wallet:                    w,
		beaconNodeClient:          mockNodeClient,
	}

	type want struct {
		epoch          primitives.Epoch
		validatorIndex uint64
		signature      []byte
	}

	tests := []struct {
		name   string
		pubkey []byte
		epoch  primitives.Epoch
		w      want
	}{
		{
			name:  "Ok: with epoch",
			epoch: 30000000,
			w: want{
				epoch:          30000000,
				validatorIndex: 2,
				signature:      []uint8{88, 156, 20, 184, 129, 195, 166, 45, 30, 118, 216, 159, 247, 255, 24, 228, 243, 46, 98, 146, 207, 175, 177, 219, 21, 21, 80, 41, 58, 174, 50, 10, 52, 38, 21, 251, 170, 29, 68, 28, 139, 49, 223, 215, 164, 14, 237, 183, 12, 37, 181, 210, 106, 226, 75, 119, 115, 178, 0, 193, 214, 230, 170, 110, 123, 222, 37, 113, 226, 5, 90, 61, 14, 160, 94, 153, 188, 124, 188, 251, 232, 241, 89, 4, 90, 127, 164, 35, 225, 220, 248, 15, 105, 36, 123, 156, 163, 137, 42, 87, 137, 188, 200, 109, 29, 143, 243, 31, 254, 165, 218, 117, 6, 170, 167, 155, 174, 66, 154, 219, 165, 151, 43, 227, 150, 92, 206, 167, 189, 173, 117, 199, 60, 155, 46, 229, 96, 104, 70, 162, 236, 26, 242, 34, 154, 138, 252, 125, 141, 226, 76, 137, 212, 0, 193, 12, 228, 88, 225, 74, 162, 159, 108, 215, 139, 236, 135, 162, 149, 183, 25, 235, 114, 171, 66, 127, 43, 12, 165, 248, 123, 102, 96, 63, 33, 123, 87, 183, 193, 14, 217, 208, 187, 146, 138, 89, 206, 41, 236, 125, 16, 4, 143, 83, 8, 70, 168, 134, 165, 29, 136, 103, 243, 147, 254, 86, 224, 115, 116, 105, 24, 239, 74, 143, 3, 244, 145, 12, 110, 7, 124, 147, 93, 90, 15, 29, 154, 118, 39, 41, 96, 41, 208, 100, 249, 143, 177, 175, 157, 31, 47, 45, 198, 27, 50, 156, 104, 127, 41, 21, 17, 67, 182, 39, 232, 172, 245, 159, 102, 208, 212, 250, 95, 132, 178, 20, 4, 231, 38, 236, 44, 125, 158, 63, 235, 34, 62, 219, 197, 158, 90, 149, 50, 160, 152, 57, 155, 251, 51, 232, 136, 182, 130, 227, 71, 137, 225, 182, 97, 184, 136, 160, 75, 73, 202, 77, 122, 54, 252, 207, 224, 239, 200, 2, 134, 233, 108, 146, 239, 2, 12, 203, 50, 150, 246, 129, 63, 34, 2, 88, 128, 107, 122, 77, 135, 14, 53, 126, 40, 233, 230, 44, 104, 194, 111, 117, 210, 102, 252, 93, 33, 170, 199, 19, 179, 151, 125, 86, 1, 44, 179, 197, 235, 240, 92, 67, 24, 131, 248, 129, 134, 69, 233, 42, 251, 20, 160, 203, 193, 81, 31, 227, 119, 58, 87, 206, 4, 28, 227, 234, 105, 9, 122, 129, 20, 144, 194, 45, 129, 112, 94, 86, 237, 137, 15, 181, 225, 155, 52, 78, 182, 135, 31, 84, 76, 242, 120, 32, 89, 102, 83, 136, 14, 67, 218, 24, 75, 195, 245, 65, 127, 54, 7, 52, 74, 195, 164, 243, 6, 228, 230, 250, 76, 146, 243, 74, 176, 201, 86, 33, 191, 221, 72, 218, 182, 136, 239, 42, 14, 69, 25, 122, 144, 64, 190, 250, 203, 172, 233, 231, 55, 86, 159, 188, 36, 146, 41, 118, 156, 109, 158, 253, 58, 252, 68, 12, 120, 42, 121, 239, 101, 205, 186, 101, 131, 192, 253, 105, 41, 22, 249, 124, 100, 156, 248, 125, 84, 85, 141, 134, 27, 231, 112, 97, 144, 113, 65, 53, 168, 165, 195, 163, 86, 248, 129, 22, 96, 171, 188, 165, 179, 139, 204, 76, 172, 8, 4, 78, 77, 28, 217, 145, 88, 249, 26, 76, 23, 196, 156, 207, 118, 198, 225, 229, 175, 124, 13, 244, 88, 64, 62, 14, 88, 82, 1, 163, 70, 180, 178, 214, 246, 167, 137, 140, 118, 163, 24, 131, 91, 116, 65, 125, 184, 2, 217, 20, 166, 74, 154, 180, 193, 16, 155, 16, 107, 118, 58, 21, 123, 113, 33, 144, 225, 248, 152, 79, 138, 134, 205, 224, 185, 37, 246, 26, 60, 4, 122, 233, 68, 165, 214, 21, 205, 249, 71, 103, 40, 11, 120, 71, 177, 203, 212, 160, 50, 252, 235, 55, 110, 24, 147, 152, 54, 124, 184, 213, 240, 86, 48, 179, 193, 149, 66, 203, 231, 35, 20, 148, 111, 122, 194, 69, 36, 15, 52, 29, 255, 66, 70, 234, 87, 26, 240, 140, 133, 190, 159, 67, 239, 246, 164, 54, 159, 156, 169, 119, 58, 242, 141, 195, 32, 89, 67, 195, 133, 220, 175, 205, 79, 155, 24, 142, 35, 3, 187, 0, 92, 201, 104, 213, 104, 91, 56, 228, 71, 172, 180, 118, 246, 143, 100, 169, 242, 236, 134, 145, 98, 134, 89, 181, 25, 114, 155, 82, 156, 135, 76, 142, 125, 178, 210, 61, 221, 230, 80, 75, 57, 21, 85, 33, 0, 105, 26, 148, 189, 156, 173, 48, 157, 212, 151, 60, 242, 47, 146, 153, 247, 50, 228, 132, 177, 7, 23, 174, 124, 80, 105, 5, 171, 199, 148, 185, 69, 157, 184, 140, 30, 146, 236, 99, 199, 66, 135, 163, 14, 93, 225, 84, 23, 109, 2, 11, 121, 137, 204, 228, 16, 34, 1, 87, 54, 148, 55, 202, 251, 231, 220, 111, 155, 193, 167, 201, 161, 194, 128, 14, 19, 1, 29, 184, 88, 234, 249, 167, 38, 129, 243, 187, 163, 150, 55, 24, 165, 95, 143, 217, 150, 120, 21, 124, 252, 181, 232, 137, 16, 56, 166, 222, 44, 250, 41, 80, 245, 10, 96, 185, 68, 155, 136, 44, 213, 179, 114, 0, 65, 73, 137, 75, 27, 10, 143, 56, 175, 211, 187, 254, 202, 8, 57, 200, 41, 18, 103, 64, 181, 25, 37, 39, 214, 110, 196, 21, 17, 96, 222, 96, 156, 73, 61, 122, 183, 5, 243, 65, 146, 21, 124, 112, 60, 56, 202, 136, 167, 54, 60, 204, 41, 75, 183, 118, 126, 63, 208, 53, 26, 110, 245, 117, 189, 230, 202, 188, 60, 82, 93, 145, 145, 235, 244, 96, 7, 156, 216, 63, 167, 61, 56, 104, 159, 112, 222, 11, 109, 233, 133, 34, 26, 230, 99, 186, 239, 16, 187, 50, 36, 215, 20, 62, 110, 11, 110, 63, 241, 22, 54, 74, 53, 1, 13, 65, 78, 67, 116, 255, 162, 210, 193, 129, 104, 155, 187, 99, 70, 127, 255, 54, 120, 195, 63, 206, 165, 186, 71, 238, 109, 184, 143, 48, 182, 228, 89, 135, 171, 136, 44, 131, 74, 89, 64, 25, 162, 197, 31, 85, 159, 191, 176, 11, 83, 70, 67, 221, 36, 174, 45, 159, 43, 82, 124, 178, 254, 241, 90, 214, 155, 153, 191, 42, 59, 252, 65, 220, 41, 227, 178, 237, 199, 238, 110, 188, 124, 219, 206, 187, 233, 23, 161, 161, 245, 192, 21, 213, 53, 50, 46, 192, 12, 30, 117, 31, 34, 213, 21, 142, 136, 173, 51, 130, 69, 99, 203, 193, 234, 3, 119, 114, 153, 22, 254, 231, 107, 185, 11, 248, 176, 35, 82, 174, 91, 149, 117, 85, 213, 136, 23, 148, 248, 232, 234, 47, 196, 128, 130, 162, 127, 55, 117, 239, 131, 222, 101, 175, 153, 108, 127, 207, 2, 160, 123, 14, 105, 73, 233, 6, 226, 200, 207, 179, 108, 12, 146, 112, 54, 31, 191, 187, 95, 188, 218, 17, 189, 7, 139, 14, 4, 1, 234, 18, 206, 7, 116, 10, 72, 79, 1, 22, 3, 32, 122, 208, 107, 179, 62, 193, 237, 72, 231, 191, 246, 71, 143, 139, 96, 85, 127, 153, 239, 141, 172, 138, 7, 34, 165, 88, 5, 178, 37, 217, 134, 69, 105, 142, 205, 127, 118, 142, 130, 11, 229, 169, 56, 100, 249, 226, 49, 185, 17, 254, 138, 41, 237, 196, 126, 151, 98, 179, 90, 8, 143, 89, 124, 134, 82, 166, 216, 84, 59, 248, 38, 63, 154, 14, 250, 109, 175, 139, 225, 171, 11, 115, 188, 74, 245, 180, 2, 128, 1, 64, 236, 134, 198, 222, 20, 37, 182, 118, 249, 191, 29, 211, 144, 97, 148, 157, 220, 246, 119, 238, 221, 240, 109, 100, 220, 147, 134, 124, 8, 226, 47, 142, 47, 154, 165, 91, 216, 246, 222, 20, 184, 0, 212, 37, 183, 116, 112, 210, 85, 62, 249, 186, 152, 82, 66, 24, 101, 140, 179, 158, 140, 242, 226, 175, 180, 34, 165, 171, 111, 139, 169, 240, 142, 73, 196, 53, 21, 116, 210, 160, 86, 67, 20, 222, 141, 205, 89, 49, 253, 136, 248, 91, 190, 219, 213, 4, 230, 61, 183, 104, 35, 51, 143, 46, 186, 79, 126, 137, 243, 106, 90, 155, 33, 213, 40, 113, 155, 244, 205, 169, 115, 8, 16, 59, 178, 23, 217, 132, 171, 248, 216, 16, 13, 210, 97, 195, 164, 228, 176, 124, 94, 200, 239, 248, 248, 122, 63, 170, 207, 68, 2, 42, 213, 168, 152, 89, 68, 53, 255, 41, 19, 30, 71, 42, 217, 130, 192, 57, 7, 123, 19, 46, 156, 238, 250, 141, 104, 3, 187, 60, 21, 28, 232, 211, 54, 16, 41, 138, 62, 251, 159, 162, 221, 126, 132, 204, 235, 168, 23, 80, 149, 22, 7, 225, 26, 49, 220, 60, 235, 115, 138, 115, 77, 218, 67, 172, 58, 153, 176, 168, 46, 139, 204, 52, 111, 176, 33, 126, 251, 210, 65, 107, 147, 142, 168, 222, 239, 176, 243, 88, 235, 151, 114, 152, 7, 2, 69, 129, 44, 172, 112, 52, 13, 216, 162, 179, 76, 172, 31, 112, 12, 199, 96, 81, 27, 50, 72, 208, 69, 215, 189, 92, 248, 27, 91, 198, 25, 163, 106, 246, 166, 108, 199, 159, 147, 9, 10, 12, 103, 35, 187, 58, 10, 183, 188, 117, 36, 104, 7, 141, 121, 76, 228, 163, 139, 66, 241, 103, 33, 159, 152, 59, 17, 239, 100, 129, 180, 95, 244, 202, 59, 211, 181, 188, 52, 15, 108, 235, 69, 59, 36, 138, 65, 104, 163, 112, 11, 24, 225, 72, 71, 172, 252, 68, 96, 14, 165, 7, 51, 124, 104, 127, 31, 142, 191, 165, 42, 155, 156, 65, 194, 100, 89, 27, 61, 182, 156, 104, 50, 28, 246, 214, 197, 235, 24, 131, 18, 80, 238, 175, 39, 212, 67, 129, 105, 18, 196, 145, 207, 55, 39, 112, 82, 135, 179, 53, 201, 208, 152, 25, 85, 248, 72, 91, 95, 93, 171, 100, 63, 251, 189, 144, 54, 68, 72, 22, 198, 175, 192, 199, 5, 227, 1, 25, 46, 248, 216, 84, 170, 253, 58, 219, 233, 212, 108, 184, 240, 34, 118, 227, 67, 21, 94, 90, 125, 121, 222, 41, 179, 41, 220, 52, 171, 187, 179, 47, 128, 44, 158, 194, 168, 141, 153, 210, 151, 83, 9, 152, 104, 154, 39, 55, 44, 114, 234, 180, 151, 235, 68, 147, 79, 108, 90, 109, 169, 248, 153, 241, 162, 107, 212, 147, 218, 196, 35, 128, 39, 161, 178, 134, 28, 228, 105, 218, 234, 41, 41, 22, 49, 144, 213, 39, 108, 86, 82, 136, 85, 98, 33, 102, 39, 112, 9, 15, 107, 12, 75, 193, 243, 157, 130, 15, 122, 76, 118, 164, 116, 151, 97, 94, 154, 57, 40, 83, 199, 1, 40, 121, 160, 156, 180, 165, 114, 231, 58, 155, 227, 96, 219, 92, 228, 232, 65, 160, 59, 93, 154, 164, 15, 16, 154, 142, 57, 36, 51, 238, 76, 184, 226, 110, 77, 60, 28, 23, 59, 113, 146, 148, 185, 139, 173, 239, 235, 154, 146, 26, 201, 22, 116, 158, 143, 44, 65, 229, 135, 26, 108, 31, 24, 194, 34, 190, 36, 186, 77, 32, 180, 94, 52, 72, 89, 221, 108, 205, 41, 59, 124, 71, 132, 35, 219, 24, 244, 118, 195, 192, 22, 114, 60, 50, 202, 31, 127, 189, 28, 24, 102, 49, 155, 191, 3, 20, 249, 184, 114, 14, 144, 94, 65, 240, 166, 77, 199, 173, 71, 81, 162, 12, 8, 208, 213, 88, 207, 93, 114, 167, 29, 62, 204, 219, 193, 138, 215, 141, 178, 56, 43, 70, 61, 239, 109, 227, 2, 0, 34, 54, 86, 116, 242, 95, 92, 158, 231, 14, 167, 9, 208, 20, 253, 81, 94, 239, 26, 134, 212, 130, 80, 170, 68, 141, 254, 67, 73, 16, 3, 165, 153, 253, 100, 162, 173, 232, 56, 173, 0, 216, 40, 219, 67, 231, 39, 164, 184, 197, 13, 109, 108, 24, 206, 120, 190, 90, 197, 239, 128, 65, 103, 149, 123, 187, 184, 127, 213, 24, 147, 167, 117, 174, 69, 146, 87, 241, 211, 191, 24, 174, 106, 3, 226, 92, 175, 112, 120, 97, 244, 43, 79, 25, 26, 48, 241, 101, 101, 125, 201, 12, 220, 40, 170, 208, 146, 26, 234, 195, 14, 57, 171, 211, 209, 68, 51, 211, 77, 203, 128, 164, 107, 201, 200, 215, 39, 247, 10, 42, 74, 238, 44, 176, 107, 53, 18, 234, 147, 19, 198, 250, 33, 142, 219, 240, 125, 197, 120, 244, 59, 63, 197, 97, 99, 177, 45, 46, 69, 211, 38, 44, 117, 119, 122, 86, 164, 6, 128, 125, 241, 82, 243, 157, 124, 130, 204, 17, 83, 157, 1, 139, 226, 180, 61, 173, 144, 166, 233, 174, 85, 160, 192, 186, 158, 161, 167, 77, 95, 86, 92, 74, 114, 29, 136, 90, 133, 98, 106, 104, 231, 243, 42, 141, 92, 185, 36, 62, 29, 180, 157, 146, 9, 68, 159, 154, 250, 120, 107, 35, 22, 32, 41, 115, 243, 195, 62, 52, 164, 167, 231, 152, 111, 133, 199, 111, 188, 115, 245, 179, 172, 238, 107, 224, 171, 11, 181, 26, 81, 19, 155, 255, 205, 249, 167, 64, 32, 140, 101, 192, 103, 239, 173, 107, 168, 46, 118, 12, 49, 254, 43, 125, 183, 164, 116, 139, 68, 13, 197, 21, 108, 69, 125, 58, 21, 61, 178, 228, 50, 82, 52, 73, 196, 129, 123, 53, 202, 182, 43, 78, 171, 168, 191, 212, 81, 45, 0, 41, 230, 105, 199, 3, 108, 231, 185, 232, 146, 12, 172, 212, 230, 239, 170, 8, 157, 217, 100, 164, 230, 88, 195, 156, 86, 210, 27, 106, 104, 96, 190, 136, 149, 30, 190, 178, 5, 224, 8, 220, 171, 25, 78, 125, 25, 224, 157, 51, 69, 232, 216, 62, 156, 47, 28, 226, 206, 208, 76, 166, 67, 237, 224, 49, 200, 38, 49, 1, 127, 155, 220, 172, 157, 250, 35, 111, 118, 254, 149, 226, 168, 249, 229, 65, 61, 231, 140, 224, 66, 232, 131, 43, 53, 47, 114, 33, 50, 242, 104, 11, 112, 228, 56, 113, 127, 45, 13, 124, 186, 114, 21, 174, 123, 45, 209, 181, 235, 146, 232, 115, 129, 239, 221, 74, 11, 137, 39, 120, 160, 244, 213, 248, 170, 133, 35, 35, 231, 221, 119, 166, 138, 253, 100, 248, 251, 15, 54, 250, 194, 164, 154, 70, 160, 155, 225, 153, 79, 205, 92, 159, 247, 180, 109, 77, 9, 11, 202, 146, 153, 220, 164, 80, 221, 47, 73, 244, 223, 128, 209, 69, 15, 23, 85, 120, 135, 69, 77, 204, 183, 66, 194, 90, 123, 32, 58, 14, 52, 73, 55, 118, 105, 51, 35, 10, 131, 103, 121, 243, 76, 254, 197, 143, 141, 63, 6, 99, 14, 255, 116, 67, 18, 147, 134, 161, 35, 198, 144, 150, 208, 208, 149, 85, 111, 3, 174, 116, 53, 176, 244, 159, 137, 212, 60, 170, 204, 94, 25, 107, 185, 252, 160, 68, 253, 147, 65, 125, 81, 153, 131, 20, 82, 24, 157, 16, 6, 27, 176, 123, 213, 154, 128, 177, 164, 76, 73, 63, 1, 204, 50, 72, 116, 85, 72, 155, 38, 222, 156, 1, 75, 29, 64, 25, 35, 241, 242, 76, 50, 55, 14, 124, 121, 164, 188, 157, 96, 224, 244, 22, 177, 225, 242, 237, 128, 226, 113, 151, 67, 70, 88, 160, 150, 249, 237, 249, 241, 224, 3, 166, 19, 235, 147, 249, 254, 104, 188, 11, 57, 218, 138, 107, 160, 94, 28, 236, 49, 22, 210, 190, 174, 44, 207, 50, 214, 188, 225, 162, 172, 49, 171, 123, 42, 241, 49, 208, 103, 78, 22, 132, 100, 207, 29, 247, 237, 127, 248, 212, 137, 189, 176, 77, 207, 19, 163, 63, 31, 44, 23, 5, 52, 92, 50, 136, 196, 101, 177, 197, 236, 222, 78, 156, 168, 57, 192, 83, 168, 173, 55, 105, 153, 224, 9, 90, 206, 159, 188, 19, 124, 163, 240, 94, 136, 2, 34, 175, 178, 9, 81, 101, 201, 142, 180, 82, 120, 62, 66, 86, 81, 229, 186, 221, 232, 201, 236, 66, 124, 50, 222, 95, 228, 164, 149, 109, 229, 241, 202, 2, 4, 166, 143, 57, 177, 23, 124, 166, 42, 161, 103, 37, 242, 241, 156, 131, 88, 47, 178, 76, 253, 95, 27, 167, 105, 52, 105, 59, 202, 179, 29, 128, 214, 46, 204, 166, 176, 207, 109, 117, 5, 202, 89, 129, 48, 180, 91, 48, 124, 220, 229, 239, 92, 66, 193, 222, 223, 49, 250, 119, 61, 115, 182, 51, 64, 131, 233, 192, 28, 68, 5, 106, 252, 158, 80, 87, 46, 213, 150, 4, 134, 55, 170, 132, 60, 61, 20, 165, 146, 197, 110, 113, 210, 229, 236, 233, 90, 229, 196, 42, 119, 24, 65, 250, 8, 61, 107, 216, 246, 250, 30, 171, 190, 90, 81, 209, 50, 51, 57, 0, 181, 13, 232, 39, 57, 47, 152, 3, 92, 23, 80, 136, 23, 42, 103, 108, 199, 173, 189, 136, 28, 27, 214, 56, 33, 88, 62, 194, 51, 113, 253, 70, 253, 232, 248, 200, 106, 154, 114, 118, 59, 195, 39, 220, 121, 98, 99, 47, 225, 247, 104, 251, 173, 38, 57, 72, 125, 166, 146, 98, 32, 245, 246, 167, 182, 158, 116, 118, 170, 17, 165, 117, 48, 99, 9, 113, 97, 24, 247, 6, 167, 20, 94, 203, 224, 211, 120, 119, 17, 239, 112, 238, 50, 87, 17, 138, 244, 209, 112, 65, 64, 129, 93, 58, 81, 167, 158, 129, 229, 32, 148, 221, 71, 130, 175, 78, 229, 229, 21, 207, 91, 244, 14, 71, 109, 190, 239, 211, 92, 225, 210, 92, 19, 234, 219, 32, 243, 43, 171, 67, 254, 111, 175, 65, 243, 247, 182, 84, 86, 164, 241, 169, 158, 180, 84, 111, 90, 90, 134, 31, 64, 237, 95, 222, 159, 34, 114, 35, 40, 72, 167, 135, 75, 119, 143, 64, 181, 233, 4, 164, 45, 35, 5, 35, 145, 131, 16, 159, 227, 155, 85, 99, 171, 255, 87, 200, 51, 254, 158, 228, 218, 116, 2, 109, 32, 91, 232, 117, 140, 223, 158, 109, 125, 93, 166, 225, 160, 90, 118, 196, 229, 138, 225, 144, 185, 130, 197, 164, 2, 90, 156, 184, 24, 91, 157, 242, 102, 245, 129, 36, 23, 58, 36, 254, 201, 14, 226, 111, 90, 87, 240, 185, 47, 155, 230, 72, 83, 3, 204, 205, 163, 180, 33, 163, 210, 147, 45, 182, 45, 175, 59, 235, 34, 17, 155, 118, 202, 87, 249, 67, 55, 137, 137, 42, 248, 110, 26, 95, 22, 246, 112, 108, 33, 136, 47, 115, 0, 33, 74, 218, 81, 0, 181, 100, 55, 34, 197, 199, 40, 132, 24, 107, 117, 181, 68, 188, 137, 86, 35, 185, 238, 27, 23, 181, 0, 132, 63, 141, 47, 252, 174, 88, 244, 25, 152, 164, 133, 17, 7, 102, 109, 40, 126, 155, 64, 108, 211, 137, 148, 128, 78, 179, 227, 123, 215, 89, 161, 181, 45, 163, 57, 13, 211, 239, 16, 92, 160, 37, 206, 32, 96, 105, 145, 79, 10, 249, 69, 53, 10, 34, 239, 138, 245, 26, 37, 204, 157, 145, 245, 136, 138, 131, 37, 148, 118, 207, 93, 33, 222, 65, 72, 88, 212, 247, 199, 43, 42, 35, 94, 95, 128, 117, 54, 179, 145, 34, 154, 13, 119, 9, 54, 220, 209, 67, 219, 42, 243, 242, 135, 203, 178, 243, 88, 26, 71, 106, 191, 200, 41, 6, 27, 69, 104, 82, 96, 24, 29, 90, 151, 7, 71, 224, 25, 50, 3, 10, 48, 68, 232, 135, 3, 242, 126, 228, 187, 139, 207, 244, 54, 98, 81, 247, 140, 250, 54, 234, 56, 135, 97, 22, 250, 152, 129, 38, 27, 186, 107, 105, 75, 206, 135, 188, 239, 180, 187, 50, 219, 154, 228, 128, 225, 162, 42, 244, 139, 245, 78, 202, 50, 140, 142, 126, 80, 36, 166, 65, 7, 199, 243, 180, 210, 228, 13, 183, 104, 77, 157, 243, 46, 102, 124, 131, 126, 29, 37, 157, 119, 43, 91, 152, 143, 127, 42, 80, 220, 159, 141, 4, 126, 250, 111, 96, 105, 135, 119, 70, 55, 162, 83, 31, 61, 129, 80, 20, 36, 40, 153, 41, 78, 142, 70, 121, 82, 14, 90, 82, 144, 38, 61, 203, 43, 14, 13, 251, 69, 7, 131, 177, 215, 145, 45, 30, 171, 50, 111, 30, 138, 111, 127, 54, 135, 246, 83, 184, 80, 119, 153, 49, 137, 5, 202, 124, 116, 235, 162, 226, 59, 9, 57, 190, 195, 87, 244, 232, 96, 47, 160, 210, 139, 73, 101, 36, 200, 10, 220, 170, 68, 211, 247, 144, 215, 55, 219, 90, 49, 157, 230, 82, 56, 86, 182, 68, 215, 174, 78, 169, 216, 39, 46, 214, 19, 147, 104, 145, 73, 14, 49, 210, 232, 227, 172, 67, 39, 57, 102, 181, 27, 34, 190, 49, 149, 223, 214, 192, 191, 248, 14, 147, 114, 122, 201, 88, 40, 146, 90, 167, 143, 185, 173, 129, 83, 137, 118, 226, 68, 117, 49, 182, 74, 75, 17, 150, 44, 69, 40, 188, 244, 232, 167, 21, 206, 199, 92, 169, 215, 237, 113, 15, 220, 203, 47, 177, 183, 56, 249, 67, 69, 170, 148, 96, 146, 175, 122, 96, 192, 186, 196, 56, 86, 240, 214, 12, 200, 229, 173, 196, 169, 58, 173, 189, 102, 189, 154, 196, 150, 223, 29, 34, 15, 178, 116, 250, 84, 244, 7, 115, 120, 239, 53, 32, 116, 165, 206, 1, 97, 2, 136, 81, 99, 139, 210, 205, 154, 216, 181, 98, 90, 107, 125, 229, 252, 49, 50, 193, 245, 186, 14, 108, 117, 138, 146, 160, 242, 86, 119, 56, 193, 179, 24, 135, 221, 218, 76, 38, 126, 73, 27, 212, 98, 148, 212, 42, 12, 251, 217, 209, 68, 170, 5, 162, 26, 195, 95, 0, 48, 230, 204, 167, 50, 53, 216, 228, 25, 80, 138, 241, 133, 59, 169, 219, 185, 135, 109, 154, 106, 182, 205, 176, 19, 94, 231, 243, 132, 209, 96, 77, 70, 90, 215, 146, 105, 180, 7, 60, 39, 131, 108, 90, 183, 247, 229, 228, 157, 168, 63, 6, 78, 70, 66, 212, 122, 36, 0, 232, 14, 192, 137, 210, 250, 112, 148, 90, 63, 175, 243, 180, 235, 248, 84, 56, 195, 192, 172, 139, 222, 135, 137, 168, 167, 185, 72, 180, 95, 145, 162, 247, 200, 45, 42, 229, 59, 18, 131, 182, 215, 176, 15, 95, 92, 85, 214, 214, 29, 145, 77, 223, 49, 109, 106, 134, 67, 10, 152, 162, 32, 120, 21, 35, 145, 87, 131, 168, 171, 225, 20, 57, 42, 123, 87, 43, 158, 40, 126, 8, 121, 67, 165, 110, 253, 91, 21, 182, 97, 240, 112, 14, 62, 54, 152, 211, 186, 239, 210, 173, 48, 95, 155, 48, 189, 66, 32, 136, 5, 49, 11, 77, 204, 253, 80, 26, 137, 181, 24, 187, 33, 42, 89, 191, 153, 195, 96, 18, 140, 20, 138, 199, 90, 163, 83, 175, 18, 175, 150, 244, 91, 118, 215, 235, 54, 116, 202, 202, 244, 161, 31, 13, 81, 41, 69, 112, 243, 186, 221, 255, 187, 161, 58, 181, 141, 58, 53, 40, 212, 242, 91, 50, 11, 219, 14, 109, 48, 3, 136, 130, 194, 188, 222, 12, 248, 237, 249, 191, 192, 44, 140, 161, 116, 212, 115, 162, 146, 65, 82, 198, 238, 99, 220, 172, 115, 0, 249, 5, 195, 166, 120, 172, 193, 184, 225, 43, 3, 192, 199, 30, 3, 247, 244, 201, 17, 80, 98, 153, 165, 84, 118, 221, 190, 122, 21, 39, 59, 244, 93, 228, 97, 149, 71, 74, 37, 181, 253, 106, 254, 137, 52, 54, 210, 255, 88, 78, 149, 176, 224, 63, 28, 12, 196, 110, 20, 88, 102, 184, 131, 153, 125, 133, 176, 216, 7, 140, 68, 114, 105, 48, 48, 197, 115, 16, 26, 159, 133, 97, 112, 208, 239, 29, 152, 105, 230, 245, 148, 20, 94, 247, 246, 105, 240, 154, 139, 194, 234, 43, 106, 140, 47, 78, 29, 84, 27, 219, 95, 65, 39, 86, 32, 227, 123, 6, 108, 109, 133, 27, 162, 148, 206, 164, 159, 175, 148, 104, 133, 35, 223, 136, 75, 152, 129, 126, 251, 1, 28, 231, 186, 134, 125, 233, 89, 252, 231, 230, 114, 89, 204, 34, 169, 198, 24, 107, 178, 133, 171, 238, 165, 26, 204, 248, 132, 101, 230, 95, 210, 110, 151, 71, 68, 205, 110, 74, 63, 195, 167, 195, 11, 121, 183, 93, 23, 71, 82, 82, 254, 171, 92, 130, 43, 86, 145, 244, 179, 47, 56, 171, 219, 84, 82, 173, 24, 206, 241, 149, 172, 14, 150, 238, 94, 191, 177, 49, 52, 18, 161, 210, 113, 42, 123, 177, 173, 21, 232, 202, 181, 113, 172, 186, 242, 181, 110, 37, 28, 146, 1, 207, 73, 43, 136, 171, 43, 115, 217, 65, 5, 19, 41, 29, 132, 18, 54, 38, 30, 46, 99, 132, 89, 170, 92, 48, 200, 245, 29, 219, 159, 225, 88, 144, 15, 242, 61, 32, 56, 49, 57, 237, 6, 5, 214, 224, 178, 82, 228, 110, 144, 22, 155, 178, 218, 108, 60, 19, 209, 33, 231, 219, 121, 123, 245, 96, 144, 8, 31, 36, 75, 158, 131, 40, 167, 211, 66, 87, 97, 30, 90, 45, 65, 152, 43, 174, 181, 245, 231, 232, 179, 213, 121, 236, 77, 17, 130, 27, 87, 24, 112, 152, 30, 181, 67, 17, 208, 197, 158, 237, 137, 199, 226, 4, 54, 116, 162, 110, 116, 216, 252, 226, 14, 32, 70, 190, 17, 0, 44, 194, 18, 223, 81, 66, 88, 29, 60, 54, 90, 19, 182, 197, 221, 194, 207, 149, 254, 199, 251, 159, 177, 140, 63, 142, 153, 85, 5, 152, 160, 122, 14, 202, 228, 81, 231, 215, 178, 45, 98, 45, 40, 185, 200, 62, 162, 28, 93, 66, 86, 28, 11, 88, 131, 130, 78, 158, 163, 167, 12, 195, 1, 224, 215, 106, 218, 14, 10, 156, 171, 253, 243, 101, 182, 86, 192, 22, 195, 95, 11, 89, 241, 89, 154, 3, 181, 7, 45, 228, 236, 17, 32, 102, 116, 149, 113, 240, 91, 54, 5, 14, 244, 126, 43, 17, 236, 35, 58, 46, 29, 34, 170, 246, 126, 69, 166, 151, 233, 124, 146, 87, 215, 12, 233, 87, 187, 148, 231, 12, 35, 156, 229, 165, 166, 46, 53, 189, 76, 233, 17, 5, 158, 230, 111, 159, 51, 40, 2, 197, 75, 177, 0, 224, 239, 82, 239, 80, 122, 252, 100, 208, 134, 172, 32, 227, 105, 157, 148, 163, 196, 4, 48, 38, 214, 71, 0, 193, 40, 254, 155, 151, 58, 199, 83, 35, 220, 106, 221, 241, 46, 109, 122, 126, 134, 167, 185, 205, 69, 83, 142, 147, 202, 225, 8, 19, 72, 127, 219, 225, 228, 16, 126, 145, 157, 216, 221, 231, 234, 244, 90, 110, 202, 229, 47, 72, 77, 170, 175, 181, 196, 206, 230, 85, 97, 202, 206, 228, 234, 36, 53, 109, 128, 183, 211, 241, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 14, 21, 30, 34, 43, 49, 56},
			},
		},
		{
			name: "Ok: epoch not set",
			w: want{
				epoch:          0,
				validatorIndex: 2,
				signature:      []uint8{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := s.SetVoluntaryExit(ctx, &qrlpbservice.SetVoluntaryExitRequest{Pubkey: pubKeys[0][:], Epoch: tt.epoch})
			require.NoError(t, err)
			if tt.w.epoch == 0 {
				genesisResponse, err := s.beaconNodeClient.GetGenesis(ctx, &emptypb.Empty{})
				require.NoError(t, err)
				tt.w.epoch, err = client.CurrentEpoch(genesisResponse.GenesisTime)
				require.NoError(t, err)
			}
			require.Equal(t, uint64(tt.w.epoch), resp.Data.Message.Epoch)
			require.Equal(t, tt.w.validatorIndex, resp.Data.Message.ValidatorIndex)
			require.NotEmpty(t, resp.Data.Signature)
		})
	}
}
