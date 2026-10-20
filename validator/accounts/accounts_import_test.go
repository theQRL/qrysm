package accounts

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrlpbservice "github.com/theQRL/qrysm/proto/qrl/service"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/validator/accounts/iface"
	"github.com/theQRL/qrysm/validator/keymanager"
	"github.com/theQRL/qrysm/validator/keymanager/local"
)

func TestImportAccounts_NoPassword(t *testing.T) {
	local.ResetCaches()
	walletDir, passwordsDir, passwordFilePath := setupWalletAndPasswordsDir(t)
	keysDir := filepath.Join(t.TempDir(), "keysDir")
	require.NoError(t, os.MkdirAll(keysDir, os.ModePerm))

	cliCtx := setupWalletCtx(t, &testWalletConfig{
		walletDir:           walletDir,
		passwordsDir:        passwordsDir,
		keysDir:             keysDir,
		keymanagerKind:      keymanager.Local,
		walletPasswordFile:  passwordFilePath,
		accountPasswordFile: passwordFilePath,
	})
	opts := []Option{
		WithWalletDir(walletDir),
		WithKeymanagerType(keymanager.Local),
		WithWalletPassword(password),
	}
	acc, err := NewCLIManager(opts...)
	require.NoError(t, err)
	w, err := acc.WalletCreate(cliCtx.Context)
	require.NoError(t, err)
	km, err := w.InitializeKeymanager(cliCtx.Context, iface.InitKeymanagerConfig{ListenForChanges: false})
	require.NoError(t, err)
	importer, ok := km.(keymanager.Importer)
	require.Equal(t, true, ok)
	resp, err := ImportAccounts(context.Background(), &ImportAccountsConfig{
		Keystores:       []*keymanager.Keystore{{}},
		Importer:        importer,
		AccountPassword: "",
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(resp))
	require.Equal(t, resp[0].Status, qrlpbservice.ImportedKeystoreStatus_ERROR)
}

func Test_importPrivateKeyAsAccount(t *testing.T) {
	walletDir, _, passwordFilePath := setupWalletAndPasswordsDir(t)
	privKeyDir := filepath.Join(t.TempDir(), "privKeys")
	require.NoError(t, os.MkdirAll(privKeyDir, os.ModePerm))
	privKeyFileName := filepath.Join(privKeyDir, "privatekey.txt")

	// We create a new private key and save it to a file on disk.
	// We create a new seed and save it to a file on disk.
	var seed [field_params.MLDSA87SeedLength]uint8
	_, err := rand.Read(seed[:])
	require.NoError(t, err)
	seedHex := fmt.Sprintf("%x", seed)
	require.NoError(
		t,
		os.WriteFile(privKeyFileName, []byte(seedHex), params.BeaconIoConfig().ReadWritePermissions),
	)
	privKey, err := ml_dsa_87.SecretKeyFromSeed(seed[:])
	require.NoError(t, err)

	// We instantiate a new wallet from a cli context.
	cliCtx := setupWalletCtx(t, &testWalletConfig{
		walletDir:          walletDir,
		keymanagerKind:     keymanager.Local,
		walletPasswordFile: passwordFilePath,
		privateKeyFile:     privKeyFileName,
	})
	walletPass := "Passwordz0320$"
	opts := []Option{
		WithWalletDir(walletDir),
		WithKeymanagerType(keymanager.Local),
		WithWalletPassword(walletPass),
	}
	acc, err := NewCLIManager(opts...)
	require.NoError(t, err)
	w, err := acc.WalletCreate(cliCtx.Context)
	require.NoError(t, err)
	km, err := local.NewKeymanager(
		cliCtx.Context,
		&local.SetupConfig{
			Wallet:           w,
			ListenForChanges: false,
		},
	)
	require.NoError(t, err)
	assert.NoError(t, importPrivateKeyAsAccount(cliCtx.Context, w, km, privKeyFileName))

	// We re-instantiate the keymanager and check we now have 1 public key.
	km, err = local.NewKeymanager(
		cliCtx.Context,
		&local.SetupConfig{
			Wallet:           w,
			ListenForChanges: false,
		},
	)
	require.NoError(t, err)
	pubKeys, err := km.FetchValidatingPublicKeys(cliCtx.Context)
	require.NoError(t, err)
	require.Equal(t, 1, len(pubKeys))
	assert.DeepEqual(t, pubKeys[0], bytesutil.ToBytes2592(privKey.PublicKey().Marshal()))
}

func Test_NameToDescriptionChangeIsOK(t *testing.T) {
	jsonString := `{"version":1, "name":"hmmm"}`
	type Obj struct {
		Version     uint   `json:"version"`
		Description string `json:"description"`
	}
	a := &Obj{}
	require.NoError(t, json.Unmarshal([]byte(jsonString), a))
	require.Equal(t, a.Description, "")
}

func Test_MarshalOmitsName(t *testing.T) {
	type Obj struct {
		Version     uint   `json:"version"`
		Description string `json:"description"`
		Name        string `json:"name,omitempty"`
	}
	a := &Obj{
		Version:     1,
		Description: "hmm",
	}

	bytes, err := json.Marshal(a)
	require.NoError(t, err)
	require.Equal(t, string(bytes), `{"version":1,"description":"hmm"}`)
}
