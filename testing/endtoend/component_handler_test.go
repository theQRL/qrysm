package endtoend

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/testing/endtoend/components"
	"github.com/theQRL/qrysm/v4/testing/endtoend/helpers"
	e2e "github.com/theQRL/qrysm/v4/testing/endtoend/params"
	e2etypes "github.com/theQRL/qrysm/v4/testing/endtoend/types"
	"golang.org/x/sync/errgroup"
)

type componentHandler struct {
	t           *testing.T
	cfg         *e2etypes.E2EConfig
	ctx         context.Context
	done        func()
	group       *errgroup.Group
	keygen      e2etypes.ComponentRunner
	tracingSink e2etypes.ComponentRunner
	// web3Signer               e2etypes.ComponentRunner
	bootnode       e2etypes.ComponentRunner
	builders       e2etypes.MultipleComponentRunners
	proxies        e2etypes.MultipleComponentRunners
	executionNodes e2etypes.MultipleComponentRunners
	beaconNodes    e2etypes.MultipleComponentRunners
	validatorNodes e2etypes.MultipleComponentRunners
}

func NewComponentHandler(cfg *e2etypes.E2EConfig, t *testing.T) *componentHandler {
	ctx, done := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)

	return &componentHandler{
		ctx:   ctx,
		done:  done,
		group: g,
		cfg:   cfg,
		t:     t,
	}
}

func (c *componentHandler) setup() {
	t, config := c.t, c.cfg
	ctx, g := c.ctx, c.group
	t.Logf("Shard index: %d\n", e2e.TestParams.TestShardIndex)
	t.Logf("Starting time: %s\n", time.Now().String())
	t.Logf("Log Path: %s\n", e2e.TestParams.LogPath)

	tracingSink := components.NewTracingSink(config.TracingSinkEndpoint)
	g.Go(func() error {
		return tracingSink.Start(ctx)
	})
	c.tracingSink = tracingSink

	// var web3RemoteSigner *components.Web3RemoteSigner
	// if config.UseWeb3RemoteSigner {
	// 	web3RemoteSigner = components.NewWeb3RemoteSigner()
	// 	g.Go(func() error {
	// 		if err := web3RemoteSigner.Start(ctx); err != nil {
	// 			return errors.Wrap(err, "failed to start web3 remote signer")
	// 		}
	// 		return nil
	// 	})
	// 	c.web3Signer = web3RemoteSigner
	// }

	// Boot node.
	bootNode := components.NewBootNode()
	g.Go(func() error {
		if err := bootNode.Start(ctx); err != nil {
			return errors.Wrap(err, "failed to start bootnode")
		}
		return nil
	})
	c.bootnode = bootNode

	// Zond execution nodes.
	executionNodes := components.NewExecutionNodeSet()
	g.Go(func() error {
		if err := executionNodes.Start(ctx); err != nil {
			return errors.Wrap(err, "failed to start executions nodes")
		}
		return nil
	})
	c.executionNodes = c.executionNodes

	// if config.TestCheckpointSync {
	// 	appendDebugEndpoints(config)
	// }

	var builders *components.BuilderSet
	var proxies *components.ProxySet
	if config.UseBuilder {
		// Builder
		builders = components.NewBuilderSet()
		g.Go(func() error {
			if err := helpers.ComponentsStarted(ctx, []e2etypes.ComponentRunner{executionNodes}); err != nil {
				return errors.Wrap(err, "builders require execution nodes to run")
			}
			if err := builders.Start(ctx); err != nil {
				return errors.Wrap(err, "failed to start builders")
			}
			return nil
		})
		c.builders = builders
	} else {
		// Proxies
		proxies = components.NewProxySet()
		g.Go(func() error {
			if err := helpers.ComponentsStarted(ctx, []e2etypes.ComponentRunner{executionNodes}); err != nil {
				return errors.Wrap(err, "proxies require execution nodes to run")
			}
			if err := proxies.Start(ctx); err != nil {
				return errors.Wrap(err, "failed to start proxies")
			}
			return nil
		})
		c.proxies = proxies
	}

	// Beacon nodes.
	beaconNodes := components.NewBeaconNodes(config)
	g.Go(func() error {
		wantedComponents := []e2etypes.ComponentRunner{executionNodes, bootNode}
		if config.UseBuilder {
			wantedComponents = append(wantedComponents, builders)
		} else {
			wantedComponents = append(wantedComponents, proxies)
		}
		if err := helpers.ComponentsStarted(ctx, wantedComponents); err != nil {
			return errors.Wrap(err, "beacon nodes require proxies, execution and boot node to run")
		}
		beaconNodes.SetENR(bootNode.ENR())
		if err := beaconNodes.Start(ctx); err != nil {
			return errors.Wrap(err, "failed to start beacon nodes")
		}
		return nil
	})
	c.beaconNodes = beaconNodes

	// Validator nodes.
	validatorNodes := components.NewValidatorNodeSet(config)
	g.Go(func() error {
		comps := []e2etypes.ComponentRunner{beaconNodes}
		// if config.UseWeb3RemoteSigner {
		// 	comps = append(comps, web3RemoteSigner)
		// }
		if err := helpers.ComponentsStarted(ctx, comps); err != nil {
			return errors.Wrap(err, "validator nodes require components to run")
		}
		if err := validatorNodes.Start(ctx); err != nil {
			return errors.Wrap(err, "failed to start validator nodes")
		}
		return nil
	})
	c.validatorNodes = validatorNodes

	c.group = g
}

func (c *componentHandler) required() []e2etypes.ComponentRunner {
	requiredComponents := []e2etypes.ComponentRunner{
		c.tracingSink, c.executionNodes, c.bootnode, c.beaconNodes, c.validatorNodes,
	}
	if c.cfg.UseBuilder {
		requiredComponents = append(requiredComponents, c.builders)
	} else {
		requiredComponents = append(requiredComponents, c.proxies)
	}
	return requiredComponents
}

func (c *componentHandler) printPIDs(logger func(string, ...interface{})) {
	msg := "\nPID of components. Attach a debugger... if you dare!\n\n"

	msg += "This test PID: " + strconv.Itoa(os.Getpid()) + " (parent=" + strconv.Itoa(os.Getppid()) + ")\n"

	// Beacon chain nodes
	msg += fmt.Sprintf("Beacon chain nodes: %v\n", PIDsFromMultiComponentRunner(c.beaconNodes))
	// Validator nodes
	msg += fmt.Sprintf("Validators: %v\n", PIDsFromMultiComponentRunner(c.validatorNodes))
	// Zond nodes
	msg += fmt.Sprintf("Zond nodes: %v\n", PIDsFromMultiComponentRunner(c.executionNodes))

	logger(msg)
}

func PIDsFromMultiComponentRunner(runner e2etypes.MultipleComponentRunners) []int {
	var pids []int

	for i := 0; true; i++ {
		c, err := runner.ComponentAtIndex(i)
		if c == nil || err != nil {
			break
		}
		p := c.UnderlyingProcess()
		if p != nil {
			pids = append(pids, p.Pid)
		}
	}
	return pids
}

func appendDebugEndpoints(cfg *e2etypes.E2EConfig) {
	debug := []string{
		"--enable-debug-rpc-endpoints",
	}
	cfg.BeaconFlags = append(cfg.BeaconFlags, debug...)
}
