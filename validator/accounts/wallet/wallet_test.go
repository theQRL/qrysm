package wallet_test

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/cmd/validator/flags"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/validator/accounts/wallet"
	"github.com/theQRL/qrysm/validator/keymanager"
	"github.com/urfave/cli/v2"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)
}

func Test_Exists_RandomFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wallet")

	exists, err := wallet.Exists(path)
	require.Equal(t, false, exists)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(path+"/direct", params.BeaconIoConfig().ReadWriteExecutePermissions), "Failed to create directory")

	exists, err = wallet.Exists(path)
	require.NoError(t, err)
	require.Equal(t, true, exists)
}

func Test_IsValid_RandomFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wallet")
	valid, err := wallet.IsValid(path)
	require.NoError(t, err)
	require.Equal(t, false, valid)

	require.NoError(t, os.MkdirAll(path, params.BeaconIoConfig().ReadWriteExecutePermissions), "Failed to create directory")

	valid, err = wallet.IsValid(path)
	require.ErrorContains(t, "no wallet found", err)
	require.Equal(t, false, valid)

	walletDir := filepath.Join(path, "direct")
	require.NoError(t, os.MkdirAll(walletDir, params.BeaconIoConfig().ReadWriteExecutePermissions), "Failed to create directory")

	valid, err = wallet.IsValid(path)
	require.NoError(t, err)
	require.Equal(t, true, valid)
}

func TestOpenOrCreateNewWallet(t *testing.T) {
	walletDir := filepath.Join(t.TempDir(), "wallet")
	newDir := filepath.Join(t.TempDir(), "new")
	passwordFileDir := filepath.Join(t.TempDir(), "passwordFile")
	require.NoError(t, os.MkdirAll(passwordFileDir, params.BeaconIoConfig().ReadWriteExecutePermissions))
	passwordFilePath1 := filepath.Join(passwordFileDir, "password1.txt")
	passwordFilePath2 := filepath.Join(passwordFileDir, "password2.txt")

	tests := []struct {
		name    string
		cliCtx  *cli.Context
		want    *wallet.Wallet
		wantErr bool
	}{
		{
			name: "New Wallet",
			cliCtx: func() *cli.Context {
				app := cli.App{}
				set := flag.NewFlagSet("test", 0)
				require.NoError(t, os.MkdirAll(newDir, 0700))
				require.NoError(t, os.WriteFile(passwordFilePath1, []byte("newnewnew"), os.ModePerm))
				set.String(flags.WalletDirFlag.Name, newDir, "")
				set.String(flags.KeymanagerKindFlag.Name, keymanager.Local.String(), "")
				set.String(flags.WalletPasswordFileFlag.Name, passwordFilePath1, "")
				assert.NoError(t, set.Set(flags.KeymanagerKindFlag.Name, keymanager.Local.String()))
				assert.NoError(t, set.Set(flags.WalletPasswordFileFlag.Name, passwordFilePath1))
				return cli.NewContext(&app, set, nil)
			}(),
			want: wallet.New(&wallet.Config{
				WalletDir:      newDir,
				KeymanagerKind: keymanager.Local,
				WalletPassword: "newnewnew",
			}),
		},
		{
			name: "Existing Wallet",
			cliCtx: func() *cli.Context {
				app := cli.App{}
				set := flag.NewFlagSet("test", 0)
				set.String(flags.WalletDirFlag.Name, walletDir, "")
				set.String(flags.KeymanagerKindFlag.Name, keymanager.Local.String(), "")
				set.String(flags.WalletPasswordFileFlag.Name, passwordFilePath2, "")
				require.NoError(t, os.WriteFile(passwordFilePath2, []byte("existing"), os.ModePerm))
				w := wallet.New(&wallet.Config{
					WalletDir:      walletDir,
					KeymanagerKind: keymanager.Local,
					WalletPassword: "existing",
				})
				require.NoError(t, w.SaveWallet())
				assert.NoError(t, set.Set(flags.WalletDirFlag.Name, walletDir))
				assert.NoError(t, set.Set(flags.KeymanagerKindFlag.Name, keymanager.Local.String()))
				assert.NoError(t, set.Set(flags.WalletPasswordFileFlag.Name, passwordFilePath2))
				return cli.NewContext(&app, set, nil)
			}(),
			want: wallet.New(&wallet.Config{
				WalletDir:      walletDir,
				KeymanagerKind: keymanager.Local,
				WalletPassword: "existing",
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := wallet.OpenOrCreateNewWallet(tt.cliCtx)
			if (err != nil) != tt.wantErr {
				t.Errorf("OpenOrCreateNewWallet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OpenOrCreateNewWallet() got = %v, want %v", got, tt.want)
			}
		})
	}
}

/*
func TestWallet_InitializeKeymanager_web3Signer_HappyPath(t *testing.T) {
	w := wallet.NewWalletForWeb3Signer()
	ctx := context.Background()
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	config := iface.InitKeymanagerConfig{
		ListenForChanges: false,
		Web3SignerConfig: &remoteweb3signer.SetupConfig{
			BaseEndpoint:          "http://localhost:8545",
			GenesisValidatorsRoot: root,
			PublicKeysURL:         "http://localhost:8545/public_keys",
		},
	}
	km, err := w.InitializeKeymanager(ctx, config)
	require.NoError(t, err)
	assert.NotNil(t, km)
}

func TestWallet_InitializeKeymanager_web3Signer_nilConfig(t *testing.T) {
	w := wallet.NewWalletForWeb3Signer()
	ctx := context.Background()
	config := iface.InitKeymanagerConfig{
		ListenForChanges: false,
		Web3SignerConfig: nil,
	}
	km, err := w.InitializeKeymanager(ctx, config)
	assert.NotNil(t, err)
	assert.Equal(t, nil, km)
}
*/
