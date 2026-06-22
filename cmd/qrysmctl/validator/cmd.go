package validator

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/cmd"
	"github.com/theQRL/qrysm/cmd/validator/accounts"
	"github.com/theQRL/qrysm/cmd/validator/flags"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/runtime/tos"
	"github.com/urfave/cli/v2"
)

var (
	BeaconHostFlag = &cli.StringFlag{
		Name:  "beacon-node-host",
		Usage: "host:port for beacon node to query",
		Value: "127.0.0.1:3500",
	}

	PathFlag = &cli.StringFlag{
		Name:    "path",
		Aliases: []string{"p"},
		Usage:   "path to the signed withdrawal messages JSON",
	}

	ConfirmFlag = &cli.BoolFlag{
		Name:    "confirm",
		Aliases: []string{"c"},
		Usage: "WARNING: User confirms and accepts responsibility of all input data provided and actions for setting their withdrawal address for their validator key. " +
			"This action is not reversible and withdrawal addresses can not be changed once set.",
	}

	VerifyOnlyFlag = &cli.BoolFlag{
		Name:    "verify-only",
		Aliases: []string{"vo"},
		Usage:   "overrides withdrawal command to only verify whether requests are in the pool and does not submit withdrawal requests",
	}

	ValidatorHostFlag = &cli.StringFlag{
		Name:    "validator-host",
		Aliases: []string{"vch"},
		Usage:   "host:port for validator client.",
		Value:   "http://127.0.0.1:7500",
	}

	ProposerSettingsOutputFlag = &cli.StringFlag{
		Name:    "output-proposer-settings-path",
		Aliases: []string{"settings-path"},
		Usage:   "path to outputting a proposer settings file ( i.e. ./path/to/proposer-settings.json), file does not include builder settings and will need to be added for advanced users using those features",
	}

	WithBuilderFlag = &cli.BoolFlag{
		Name:    "with-builder",
		Aliases: []string{"wb"},
		Usage:   "adds default builder options to proposer settings output, used for enabling mev-boost and relays",
	}

	DefaultFeeRecipientFlag = &cli.StringFlag{
		Name:    "default-fee-recipient",
		Aliases: []string{"dfr"},
		Usage:   "default fee recipient used for proposer-settings, only used with --output-proposer-settings-path",
	}

	TokenFlag = &cli.StringFlag{
		Name:    "token",
		Aliases: []string{"t"},
		Usage:   "keymanager API bearer token read from <wallet-dir>/auth-token on the validator host.",
	}
)

var Commands = []*cli.Command{
	{
		Name:    "validator",
		Aliases: []string{"v"},
		Usage:   "commands that affect the state of validators such as exiting or withdrawing",
		Subcommands: []*cli.Command{
			{
				Name:    "proposer-settings",
				Aliases: []string{"w"},
				Usage:   "Display or recreate currently used proposer settings.",
				Flags: []cli.Flag{
					cmd.ConfigFileFlag,
					DefaultFeeRecipientFlag,
					TokenFlag,
					ValidatorHostFlag,
					ProposerSettingsOutputFlag,
				},
				Before: func(cliCtx *cli.Context) error {
					return cmd.LoadFlagsFromConfig(cliCtx, cliCtx.Command.Flags)
				},
				Action: func(cliCtx *cli.Context) error {
					if err := getProposerSettings(cliCtx, os.Stdin); err != nil {
						log.WithError(err).Fatal("Could not get proposer settings")
					}
					return nil
				},
			},
			{
				Name:    "exit",
				Aliases: []string{"e", "voluntary-exit"},
				Usage:   "Performs a voluntary exit on selected accounts",
				Flags: cmd.WrapFlags([]cli.Flag{
					flags.WalletDirFlag,
					flags.WalletPasswordFileFlag,
					flags.AccountPasswordFileFlag,
					flags.VoluntaryExitPublicKeysFlag,
					flags.BeaconRPCProviderFlag,
					// flags.Web3SignerURLFlag,
					// flags.Web3SignerPublicValidatorKeysFlag,
					flags.InteropNumValidators,
					flags.InteropStartIndex,
					cmd.GrpcMaxCallRecvMsgSizeFlag,
					flags.CertFlag,
					flags.GrpcHeadersFlag,
					flags.GrpcRetriesFlag,
					flags.GrpcRetryDelayFlag,
					flags.ExitAllFlag,
					flags.ForceExitFlag,
					flags.VoluntaryExitJSONOutputPath,
					features.Mainnet,
					cmd.AcceptTosFlag,
				}),
				Before: func(cliCtx *cli.Context) error {
					if err := cmd.LoadFlagsFromConfig(cliCtx, cliCtx.Command.Flags); err != nil {
						return err
					}
					if err := tos.VerifyTosAcceptedOrPrompt(cliCtx); err != nil {
						return err
					}
					return features.ConfigureValidator(cliCtx)
				},
				Action: func(cliCtx *cli.Context) error {
					if err := accounts.AccountsExit(cliCtx, os.Stdin); err != nil {
						log.WithError(err).Fatal("Could not perform voluntary exit")
					}
					return nil
				},
			},
		},
	},
}
