package generatedilithiumtoexecutionchange

import (
	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/stakingdeposit"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var (
	generateDilithiumToExecutionChangeFlags = struct {
		DilithiumToExecutionChangesFolder  string
		Chain                              string
		Seed                               string
		SeedPassword                       string
		ValidatorStartIndex                uint64
		ValidatorIndices                   *cli.Uint64Slice
		DilithiumWithdrawalCredentialsList *cli.StringSlice
		ExecutionAddress                   string
		DevnetChainSetting                 string
	}{
		ValidatorIndices:                   cli.NewUint64Slice(),
		DilithiumWithdrawalCredentialsList: cli.NewStringSlice(),
	}
	log = logrus.WithField("prefix", "deposit")
)

var Commands = []*cli.Command{
	{
		Name:    "generate-dilithium-to-execution-change",
		Aliases: []string{"generate-execution-change"},
		Usage:   "",
		Action: func(cliCtx *cli.Context) error {
			if err := cliActionGenerateDilithiumToExecutionChange(cliCtx); err != nil {
				log.WithError(err).Fatal("Could not generate using an existing seed")
			}
			return nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "dilithium-to-execution-changes-folder",
				Usage:       "Folder where the dilithium to execution changes files will be created",
				Destination: &generateDilithiumToExecutionChangeFlags.DilithiumToExecutionChangesFolder,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "chain",
				Usage:       "Name of the chain should be one of these mainnet, betanet",
				Destination: &generateDilithiumToExecutionChangeFlags.Chain,
				Value:       "mainnet",
			},
			&cli.StringFlag{
				Name:        "seed",
				Usage:       "",
				Destination: &generateDilithiumToExecutionChangeFlags.Seed,
				Required:    true,
			},
			// TODO (cyyber) : Move seed password to prompt
			&cli.StringFlag{
				Name:        "seed-password",
				Usage:       "",
				Destination: &generateDilithiumToExecutionChangeFlags.SeedPassword,
				Required:    true,
			},
			&cli.Uint64Flag{
				Name:        "validator-start-index",
				Usage:       "",
				Destination: &generateDilithiumToExecutionChangeFlags.ValidatorStartIndex,
				Required:    true,
			},
			&cli.Uint64SliceFlag{
				Name:        "validator-indices",
				Usage:       "",
				Destination: generateDilithiumToExecutionChangeFlags.ValidatorIndices,
				Required:    true,
			},
			&cli.StringSliceFlag{
				Name:        "dilithium-withdrawal-credentials-list",
				Usage:       "",
				Destination: generateDilithiumToExecutionChangeFlags.DilithiumWithdrawalCredentialsList,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "execution-address",
				Usage:       "",
				Destination: &generateDilithiumToExecutionChangeFlags.ExecutionAddress,
				Value:       "betanet",
			},
			&cli.StringFlag{
				Name:        "devnet-chain-setting",
				Usage:       "Use for devnet only, to set the custom network_name, genesis_fork_name, genesis_validator_root. Input should be in JSON format.",
				Destination: &generateDilithiumToExecutionChangeFlags.DevnetChainSetting,
				Value:       "",
			},
		},
		Subcommands: []*cli.Command{
			nil,
		},
	},
}

func cliActionGenerateDilithiumToExecutionChange(cliCtx *cli.Context) error {
	stakingdeposit.GenerateDilithiumToExecutionChange(
		generateDilithiumToExecutionChangeFlags.DilithiumToExecutionChangesFolder,
		generateDilithiumToExecutionChangeFlags.Chain,
		generateDilithiumToExecutionChangeFlags.Seed,
		generateDilithiumToExecutionChangeFlags.ValidatorStartIndex,
		generateDilithiumToExecutionChangeFlags.ValidatorIndices.Value(),
		generateDilithiumToExecutionChangeFlags.DilithiumWithdrawalCredentialsList.Value(),
		generateDilithiumToExecutionChangeFlags.ExecutionAddress,
		generateDilithiumToExecutionChangeFlags.DevnetChainSetting,
	)
	return nil
}
