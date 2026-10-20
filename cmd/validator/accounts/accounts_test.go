package accounts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/theQRL/qrysm/cmd"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/urfave/cli/v2"
)

func TestConfigureVoluntaryExit_LoadsCustomChainConfig(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	chainConfig := params.MainnetConfig().Copy()
	chainConfig.SecondsPerSlot = 6
	chainConfig.SlotsPerEpoch = 8
	chainConfigPath := filepath.Join(t.TempDir(), "chain-config.yaml")
	require.NoError(t, os.WriteFile(chainConfigPath, params.ConfigToYaml(chainConfig), 0o600))
	dataDir := t.TempDir()

	actionCalled := false
	app := voluntaryExitConfigTestApp(t, func(*cli.Context) error {
		actionCalled = true
		require.Equal(t, uint64(6), params.BeaconConfig().SecondsPerSlot)
		require.Equal(t, uint64(8), uint64(params.BeaconConfig().SlotsPerEpoch))
		return nil
	})

	err := app.Run([]string{
		"validator",
		"--chain-config-file", chainConfigPath,
		"--datadir", dataDir,
		"accounts",
		"voluntary-exit",
		"--accept-terms-of-use",
	})
	require.NoError(t, err)
	require.Equal(t, true, actionCalled)
}

func TestConfigureVoluntaryExit_RejectsMalformedChainConfig(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	chainConfigPath := filepath.Join(t.TempDir(), "chain-config.yaml")
	require.NoError(t, os.WriteFile(chainConfigPath, []byte("SECONDS_PER_SLOT: ["), 0o600))
	dataDir := t.TempDir()

	actionCalled := false
	app := voluntaryExitConfigTestApp(t, func(*cli.Context) error {
		actionCalled = true
		return nil
	})

	err := app.Run([]string{
		"validator",
		"--chain-config-file", chainConfigPath,
		"--datadir", dataDir,
		"accounts",
		"voluntary-exit",
		"--accept-terms-of-use",
	})
	require.ErrorContains(t, "Failed to parse chain config yaml file", err)
	require.Equal(t, false, actionCalled)
}

func voluntaryExitConfigTestApp(t *testing.T, action cli.ActionFunc) *cli.App {
	t.Helper()
	t.Cleanup(features.InitWithReset(features.Get()))

	var voluntaryExitCommand *cli.Command
	for _, subcommand := range Commands.Subcommands {
		if subcommand.Name == "voluntary-exit" {
			commandCopy := *subcommand
			commandCopy.Action = action
			voluntaryExitCommand = &commandCopy
			break
		}
	}
	if voluntaryExitCommand == nil {
		t.Fatal("voluntary-exit command is not registered")
	}

	accountsCommand := *Commands
	accountsCommand.Subcommands = []*cli.Command{voluntaryExitCommand}
	return &cli.App{
		Flags:    []cli.Flag{cmd.ChainConfigFileFlag, cmd.DataDirFlag},
		Commands: []*cli.Command{&accountsCommand},
	}
}
