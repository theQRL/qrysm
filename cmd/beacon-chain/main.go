// Package beacon-chain defines the entire runtime of a QRL beacon node.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	runtimeDebug "runtime/debug"
	"syscall"

	golog "github.com/ipfs/go-log/v2"
	joonix "github.com/joonix/log"
	"github.com/sirupsen/logrus"
	gqrllog "github.com/theQRL/go-qrl/log"
	"github.com/theQRL/qrysm/beacon-chain/builder"
	"github.com/theQRL/qrysm/beacon-chain/node"
	"github.com/theQRL/qrysm/cmd"
	blockchaincmd "github.com/theQRL/qrysm/cmd/beacon-chain/blockchain"
	dbcommands "github.com/theQRL/qrysm/cmd/beacon-chain/db"
	"github.com/theQRL/qrysm/cmd/beacon-chain/execution"
	"github.com/theQRL/qrysm/cmd/beacon-chain/flags"
	jwtcommands "github.com/theQRL/qrysm/cmd/beacon-chain/jwt"
	"github.com/theQRL/qrysm/cmd/beacon-chain/sync/checkpoint"
	"github.com/theQRL/qrysm/cmd/beacon-chain/sync/genesis"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/io/logs"
	"github.com/theQRL/qrysm/monitoring/journald"
	"github.com/theQRL/qrysm/runtime/debug"
	"github.com/theQRL/qrysm/runtime/fdlimits"
	prefixed "github.com/theQRL/qrysm/runtime/logging/logrus-prefixed-formatter"
	_ "github.com/theQRL/qrysm/runtime/maxprocs"
	"github.com/theQRL/qrysm/runtime/tos"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/urfave/cli/v2"
)

var appFlags = []cli.Flag{
	flags.DepositContractFlag,
	flags.ExecutionEngineEndpoint,
	flags.ExecutionEngineHeaders,
	flags.ExecutionJWTSecretFlag,
	flags.RPCHost,
	flags.RPCPort,
	flags.CertFlag,
	flags.KeyFlag,
	flags.HTTPModules,
	flags.DisableGRPCGateway,
	flags.GRPCGatewayHost,
	flags.GRPCGatewayPort,
	flags.GPRCGatewayCorsDomain,
	flags.MinSyncPeers,
	flags.ContractDeploymentBlock,
	flags.SetGCPercent,
	flags.BlockBatchLimit,
	flags.BlockBatchLimitBurstFactor,
	flags.InteropMockExecutionDataVotesFlag,
	flags.InteropNumValidatorsFlag,
	flags.InteropGenesisTimeFlag,
	flags.SlotsPerArchivedPoint,
	flags.EnableDebugRPCEndpoints,
	flags.SubscribeToAllSubnets,
	flags.HistoricalSlasherNode,
	flags.ChainID,
	flags.NetworkID,
	flags.WeakSubjectivityCheckpoint,
	flags.ExecutionHeaderReqLimit,
	flags.MinPeersPerSubnet,
	flags.MaxConcurrentDials,
	flags.SuggestedFeeRecipient,
	flags.MevRelayEndpoint,
	flags.MaxBuilderEpochMissedSlots,
	flags.MaxBuilderConsecutiveMissedSlots,
	flags.EngineEndpointTimeoutSeconds,
	flags.LocalBlockValueBoost,
	cmd.BackupWebhookOutputDir,
	cmd.MinimalConfigFlag,
	cmd.E2EConfigFlag,
	cmd.RPCMaxPageSizeFlag,
	cmd.BootstrapNode,
	cmd.NoDiscovery,
	cmd.StaticPeers,
	cmd.RelayNode,
	cmd.P2PUDPPort,
	cmd.P2PTCPPort,
	cmd.P2PIP,
	cmd.P2PHost,
	cmd.P2PHostDNS,
	cmd.P2PMaxPeers,
	cmd.P2PPrivKey,
	cmd.P2PStaticID,
	cmd.P2PAllowList,
	cmd.P2PDenyList,
	cmd.DataDirFlag,
	cmd.VerbosityFlag,
	cmd.EnableTracingFlag,
	cmd.TracingProcessNameFlag,
	cmd.TracingEndpointFlag,
	cmd.TraceSampleFractionFlag,
	cmd.MonitoringHostFlag,
	flags.MonitoringPortFlag,
	cmd.DisableMonitoringFlag,
	cmd.ClearDB,
	cmd.ForceClearDB,
	cmd.LogFormat,
	cmd.MaxGoroutines,
	debug.PProfFlag,
	debug.PProfAddrFlag,
	debug.PProfPortFlag,
	debug.MemProfileRateFlag,
	debug.BlockProfileRateFlag,
	debug.MutexProfileFractionFlag,
	cmd.LogFileName,
	cmd.EnableUPnPFlag,
	cmd.ConfigFileFlag,
	cmd.ChainConfigFileFlag,
	cmd.GrpcMaxCallRecvMsgSizeFlag,
	cmd.AcceptTosFlag,
	cmd.RestoreSourceFileFlag,
	cmd.RestoreTargetDirFlag,
	cmd.ValidatorMonitorIndicesFlag,
	cmd.ApiTimeoutFlag,
	checkpoint.BlockPath,
	checkpoint.StatePath,
	checkpoint.RemoteURL,
	genesis.StatePath,
	genesis.BeaconAPIURL,
	flags.SlasherDirFlag,
}

func init() {
	appFlags = cmd.WrapFlags(append(appFlags, features.BeaconChainFlags...))
}

func main() {
	app := cli.App{}
	app.Name = "beacon-chain"
	app.Usage = "this is a beacon chain implementation for QRL"
	app.Action = func(ctx *cli.Context) error {
		if err := startNode(ctx); err != nil {
			return cli.Exit(err.Error(), 1)
		}
		return nil
	}
	app.Version = version.Version()
	app.Commands = []*cli.Command{
		dbcommands.Commands,
		jwtcommands.Commands,
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
			// the colors are ANSI codes and seen as gibberish in the log files.
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
		if err := cmd.ExpandSingleEndpointIfFile(ctx, flags.ExecutionEngineEndpoint); err != nil {
			return err
		}
		if ctx.IsSet(flags.SetGCPercent.Name) {
			runtimeDebug.SetGCPercent(ctx.Int(flags.SetGCPercent.Name))
		}
		if err := debug.Setup(ctx); err != nil {
			return err
		}
		if err := fdlimits.SetMaxFdLimits(); err != nil {
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

	// Build a cancellable root context that's cancelled on SIGINT/SIGTERM so
	// the entire process tree can shut down via context propagation rather
	// than each subsystem having to wire its own signal handler.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("Received shutdown signal, cancelling root context")
		cancel()
	}()

	if err := app.RunContext(ctx, os.Args); err != nil {
		log.Error(err.Error())
	}
}

func startNode(ctx *cli.Context) error {
	// verify if ToS accepted
	if err := tos.VerifyTosAcceptedOrPrompt(ctx); err != nil {
		return err
	}

	verbosity := ctx.String(cmd.VerbosityFlag.Name)
	level, err := logrus.ParseLevel(verbosity)
	if err != nil {
		return err
	}
	logrus.SetLevel(level)
	// Set libp2p logger to only panic logs for the info level.
	golog.SetAllLoggers(golog.LevelPanic)

	if level == logrus.DebugLevel {
		// Set libp2p logger to error logs for the debug level.
		golog.SetAllLoggers(golog.LevelError)
	}
	if level == logrus.TraceLevel {
		// libp2p specific logging.
		golog.SetAllLoggers(golog.LevelDebug)
		// Gqrl specific logging.
		gqrllog.SetDefault(gqrllog.NewLogger(gqrllog.NewTerminalHandlerWithLevel(os.Stderr, gqrllog.LvlTrace, true)))
	}

	blockchainFlagOpts, err := blockchaincmd.FlagOptions(ctx)
	if err != nil {
		return err
	}
	executionFlagOpts, err := execution.FlagOptions(ctx)
	if err != nil {
		return err
	}
	builderFlagOpts, err := builder.FlagOptions(ctx)
	if err != nil {
		return err
	}
	opts := []node.Option{
		node.WithBlockchainFlagOptions(blockchainFlagOpts),
		node.WithExecutionChainOptions(executionFlagOpts),
		node.WithBuilderFlagOptions(builderFlagOpts),
	}

	optFuncs := []func(*cli.Context) (node.Option, error){
		genesis.BeaconNodeOptions,
		checkpoint.BeaconNodeOptions,
	}

	beacon, err := node.New(ctx, optFuncs, opts...)
	if err != nil {
		return fmt.Errorf("unable to start beacon node: %w", err)
	}
	beacon.Start()
	return nil
}
