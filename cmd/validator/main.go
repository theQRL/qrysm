// Package main defines a validator client, a critical actor in QRL which manages
// a keystore of private keys, connects to a beacon node to receive assignments,
// and submits blocks/attestations as needed.
package main

import (
	"fmt"
	"os"
	runtimeDebug "runtime/debug"

	joonix "github.com/joonix/log"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/cmd"
	accountcommands "github.com/theQRL/qrysm/cmd/validator/accounts"
	dbcommands "github.com/theQRL/qrysm/cmd/validator/db"
	"github.com/theQRL/qrysm/cmd/validator/flags"
	slashingprotectioncommands "github.com/theQRL/qrysm/cmd/validator/slashing-protection"
	walletcommands "github.com/theQRL/qrysm/cmd/validator/wallet"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/io/logs"
	"github.com/theQRL/qrysm/monitoring/journald"
	"github.com/theQRL/qrysm/runtime/debug"
	prefixed "github.com/theQRL/qrysm/runtime/logging/logrus-prefixed-formatter"
	_ "github.com/theQRL/qrysm/runtime/maxprocs"
	"github.com/theQRL/qrysm/runtime/tos"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/validator/node"
	"github.com/urfave/cli/v2"
)

func startNode(ctx *cli.Context) error {
	// verify if ToS accepted
	if err := tos.VerifyTosAcceptedOrPrompt(ctx); err != nil {
		return err
	}

	validatorClient, err := node.NewValidatorClient(ctx)
	if err != nil {
		return err
	}
	validatorClient.Start()
	return nil
}

var appFlags = []cli.Flag{
	flags.BeaconRPCProviderFlag,
	flags.BeaconRPCGatewayProviderFlag,
	flags.BeaconRESTApiProviderFlag,
	flags.CertFlag,
	flags.GraffitiFlag,
	flags.DisablePenaltyRewardLogFlag,
	flags.InteropStartIndex,
	flags.InteropNumValidators,
	flags.EnableRPCFlag,
	flags.RPCHost,
	flags.RPCPort,
	flags.GRPCGatewayPort,
	flags.GRPCGatewayHost,
	flags.GrpcRetriesFlag,
	flags.GrpcRetryDelayFlag,
	flags.GrpcHeadersFlag,
	flags.GPRCGatewayCorsDomain,
	flags.DisableAccountMetricsFlag,
	flags.MonitoringPortFlag,
	flags.SlasherRPCProviderFlag,
	flags.SlasherCertFlag,
	flags.WalletPasswordFileFlag,
	flags.WalletDirFlag,
	flags.GraffitiFileFlag,
	// Consensys' Web3Signer flags
	// flags.Web3SignerURLFlag,
	// flags.Web3SignerPublicValidatorKeysFlag,
	flags.SuggestedFeeRecipientFlag,
	flags.ProposerSettingsURLFlag,
	flags.ProposerSettingsFlag,
	flags.EnableBuilderFlag,
	flags.BuilderGasLimitFlag,
	////////////////////
	cmd.DisableMonitoringFlag,
	cmd.MonitoringHostFlag,
	cmd.BackupWebhookOutputDir,
	cmd.EnableBackupWebhookFlag,
	cmd.MinimalConfigFlag,
	cmd.E2EConfigFlag,
	cmd.VerbosityFlag,
	cmd.DataDirFlag,
	cmd.ClearDB,
	cmd.ForceClearDB,
	cmd.EnableTracingFlag,
	cmd.TracingProcessNameFlag,
	cmd.TracingEndpointFlag,
	cmd.TraceSampleFractionFlag,
	cmd.LogFormat,
	cmd.LogFileName,
	cmd.ConfigFileFlag,
	cmd.ChainConfigFileFlag,
	cmd.GrpcMaxCallRecvMsgSizeFlag,
	cmd.ApiTimeoutFlag,
	debug.PProfFlag,
	debug.PProfAddrFlag,
	debug.PProfPortFlag,
	debug.MemProfileRateFlag,
	debug.BlockProfileRateFlag,
	debug.MutexProfileFractionFlag,
	cmd.AcceptTosFlag,
}

func init() {
	appFlags = cmd.WrapFlags(append(appFlags, features.ValidatorFlags...))
}

func main() {
	app := cli.App{}
	app.Name = "validator"
	app.Usage = `launches a QRL validator client that interacts with a beacon chain, starts proposer and attester services, p2p connections, and more`
	app.Version = version.Version()
	app.Action = func(ctx *cli.Context) error {
		if err := startNode(ctx); err != nil {
			return cli.Exit(err.Error(), 1)
		}
		return nil
	}
	app.Commands = []*cli.Command{
		walletcommands.Commands,
		accountcommands.Commands,
		slashingprotectioncommands.Commands,
		dbcommands.Commands,
	}

	app.Flags = appFlags

	app.Before = func(ctx *cli.Context) error {
		// Load flags from config file, if specified.
		if err := cmd.LoadFlagsFromConfig(ctx, app.Flags); err != nil {
			return err
		}

		// Apply verbosity before installing formatters/hooks so non-text formats and the
		// journald hook honour --verbosity instead of falling back to logrus' default Info.
		level, err := logrus.ParseLevel(ctx.String(cmd.VerbosityFlag.Name))
		if err != nil {
			return err
		}
		logrus.SetLevel(level)

		format := ctx.String(cmd.LogFormat.Name)
		switch format {
		case "text":
			formatter := new(prefixed.TextFormatter)
			formatter.TimestampFormat = "2006-01-02 15:04:05"
			formatter.FullTimestamp = true
			// If persistent log files are written - we disable the log messages coloring because
			// the colors are ANSI codes and seen as Gibberish in the log files.
			formatter.DisableColors = ctx.String(cmd.LogFileName.Name) != ""
			logrus.SetFormatter(formatter)
		case "fluentd":
			f := joonix.NewFormatter()
			if err := joonix.DisableTimestampFormat(f); err != nil {
				panic(err)
			}
			logrus.SetFormatter(f)
		case "json":
			logrus.SetFormatter(&logrus.JSONFormatter{})
		case "journald":
			if err := journald.Enable(level); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown log format %s", format)
		}

		logFileName := ctx.String(cmd.LogFileName.Name)
		if logFileName != "" {
			if err := logs.ConfigurePersistentLogging(logFileName); err != nil {
				log.WithError(err).Error("Failed to configuring logging to disk.")
			}
		}

		if err := debug.Setup(ctx); err != nil {
			return err
		}
		return cmd.ValidateNoArgs(ctx)
	}

	defer func() {
		if x := recover(); x != nil {
			log.Errorf("Runtime panic: %v\n%v", x, string(runtimeDebug.Stack()))
			panic(x)
		}
	}()

	if err := app.Run(os.Args); err != nil {
		log.Error(err.Error())
	}
}
