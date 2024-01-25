package components

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/pkg/errors"
	"github.com/theQRL/go-zond/rpc"
	"github.com/theQRL/go-zond/zondclient"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/io/file"
	"github.com/theQRL/qrysm/v4/runtime/interop"
	"github.com/theQRL/qrysm/v4/testing/endtoend/helpers"
	e2e "github.com/theQRL/qrysm/v4/testing/endtoend/params"
	e2etypes "github.com/theQRL/qrysm/v4/testing/endtoend/types"
)

// ExecutionNodeSet represents a set of execution nodes, none of which is a mining node.
type ExecutionNodeSet struct {
	e2etypes.ComponentRunner
	started chan struct{}
	nodes   []e2etypes.ComponentRunner
}

// NewNodeSet creates and returns a set of zond nodes.
func NewExecutionNodeSet() *ExecutionNodeSet {
	return &ExecutionNodeSet{
		started: make(chan struct{}, 1),
	}
}

// Start starts all the execution nodes in set.
func (s *ExecutionNodeSet) Start(ctx context.Context) error {
	totalNodeCount := e2e.TestParams.BeaconNodeCount
	nodes := make([]e2etypes.ComponentRunner, totalNodeCount)
	for i := 0; i < totalNodeCount; i++ {
		node := NewExecutionNode(i)
		nodes[i] = node
	}
	s.nodes = nodes

	// Wait for all nodes to finish their job (blocking).
	// Once nodes are ready passed in handler function will be called.
	return helpers.WaitOnNodes(ctx, nodes, func() {
		// All nodes started, close channel, so that all services waiting on a set, can proceed.
		close(s.started)
	})
}

// Started checks whether execution node set is started and all nodes are ready to be queried.
func (s *ExecutionNodeSet) Started() <-chan struct{} {
	return s.started
}

// Pause pauses the component and its underlying process.
func (s *ExecutionNodeSet) Pause() error {
	for _, n := range s.nodes {
		if err := n.Pause(); err != nil {
			return err
		}
	}
	return nil
}

// Resume resumes the component and its underlying process.
func (s *ExecutionNodeSet) Resume() error {
	for _, n := range s.nodes {
		if err := n.Resume(); err != nil {
			return err
		}
	}
	return nil
}

// Stop stops the component and its underlying process.
func (s *ExecutionNodeSet) Stop() error {
	for _, n := range s.nodes {
		if err := n.Stop(); err != nil {
			return err
		}
	}
	return nil
}

// PauseAtIndex pauses the component and its underlying process at the desired index.
func (s *ExecutionNodeSet) PauseAtIndex(i int) error {
	if i >= len(s.nodes) {
		return errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.nodes))
	}
	return s.nodes[i].Pause()
}

// ResumeAtIndex resumes the component and its underlying process at the desired index.
func (s *ExecutionNodeSet) ResumeAtIndex(i int) error {
	if i >= len(s.nodes) {
		return errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.nodes))
	}
	return s.nodes[i].Resume()
}

// StopAtIndex stops the component and its underlying process at the desired index.
func (s *ExecutionNodeSet) StopAtIndex(i int) error {
	if i >= len(s.nodes) {
		return errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.nodes))
	}
	return s.nodes[i].Stop()
}

// ComponentAtIndex returns the component at the provided index.
func (s *ExecutionNodeSet) ComponentAtIndex(i int) (e2etypes.ComponentRunner, error) {
	if i >= len(s.nodes) {
		return nil, errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.nodes))
	}
	return s.nodes[i], nil
}

// Node represents a zond node.
type ExecutionNode struct {
	e2etypes.ComponentRunner
	started chan struct{}
	index   int
	enr     string
	cmd     *exec.Cmd
}

// NewExecutionNode creates and returns an execution node.
func NewExecutionNode(index int) *ExecutionNode {
	return &ExecutionNode{
		started: make(chan struct{}, 1),
		index:   index,
	}
}

// Start runs a non-mining zond node.
// To connect to a miner and start working properly, this node should be a part of a NodeSet.
func (node *ExecutionNode) Start(ctx context.Context) error {
	binaryPath, found := bazel.FindBinary("cmd/gzond", "gzond")
	if !found {
		return errors.New("go-zond binary not found")
	}

	zondPath := path.Join(e2e.TestParams.TestPath, "zonddata/"+strconv.Itoa(node.index)+"/")
	// Clear out potentially existing dir to prevent issues.
	if _, err := os.Stat(zondPath); !os.IsNotExist(err) {
		if err = os.RemoveAll(zondPath); err != nil {
			return err
		}
	}

	if err := file.MkdirAll(zondPath); err != nil {
		return err
	}
	gzondJsonPath := path.Join(zondPath, "genesis.json")

	gen := interop.GzondTestnetGenesis(e2e.TestParams.ELGenesisTime, params.BeaconConfig())
	b, err := json.Marshal(gen)
	if err != nil {
		return err
	}

	if err := file.WriteFile(gzondJsonPath, b); err != nil {
		return err
	}
	copyPath := path.Join(e2e.TestParams.LogPath, "execution-genesis.json")
	if err := file.WriteFile(copyPath, b); err != nil {
		return err
	}

	initCmd := exec.CommandContext(ctx, binaryPath, "init", fmt.Sprintf("--datadir=%s", zondPath), gzondJsonPath) // #nosec G204 -- Safe
	initFile, err := helpers.DeleteAndCreateFile(e2e.TestParams.LogPath, "execution-init_"+strconv.Itoa(node.index)+".log")
	if err != nil {
		return err
	}
	initCmd.Stderr = initFile
	if err = initCmd.Start(); err != nil {
		return err
	}
	if err = initCmd.Wait(); err != nil {
		return err
	}

	args := []string{
		"--nat=none", // disable nat traversal in e2e, it is failure prone and not needed
		fmt.Sprintf("--datadir=%s", zondPath),
		fmt.Sprintf("--http.port=%d", e2e.TestParams.Ports.GzondExecutionNodeRPCPort+node.index),
		fmt.Sprintf("--ws.port=%d", e2e.TestParams.Ports.GzondExecutionNodeWSPort+node.index),
		fmt.Sprintf("--authrpc.port=%d", e2e.TestParams.Ports.GzondExecutionNodeAuthRPCPort+node.index),
		fmt.Sprintf("--bootnodes=%s", node.enr),
		fmt.Sprintf("--port=%d", e2e.TestParams.Ports.GzondExecutionNodePort+node.index),
		fmt.Sprintf("--networkid=%d", NetworkId),
		"--http",
		"--http.api=engine,net,zond",
		"--http.addr=127.0.0.1",
		"--http.corsdomain=\"*\"",
		"--http.vhosts=\"*\"",
		"--rpc.allow-unprotected-txs",
		"--ws",
		"--ws.api=net,zond,engine",
		"--ws.addr=127.0.0.1",
		"--ws.origins=\"*\"",
		"--ipcdisable",
		"--verbosity=4",
		"--syncmode=full",
		// fmt.Sprintf("--txpool.locals=%s", EthAddress),
	}

	// give the miner start a couple of tries, since the p2p networking check is flaky
	var retryErr error
	for retries := 0; retries < 3; retries++ {
		retryErr = nil
		log.Infof("Starting execution node %d, attempt %d with flags: %s", node.index, retries, strings.Join(args[2:], " "))
		runCmd := exec.CommandContext(ctx, binaryPath, args...) // #nosec G204 -- Safe
		errLog, err := os.Create(path.Join(e2e.TestParams.LogPath, "execution_"+strconv.Itoa(node.index)+".log"))
		if err != nil {
			return err
		}
		runCmd.Stderr = errLog
		if err = runCmd.Start(); err != nil {
			return fmt.Errorf("failed to start zond chain: %w", err)
		}
		if err = helpers.WaitForTextInFile(errLog, "Started P2P networking"); err != nil {
			kerr := runCmd.Process.Kill()
			if kerr != nil {
				log.WithError(kerr).Error("error sending kill to failed node command process")
			}
			retryErr = fmt.Errorf("P2P log not found, this means the zond chain had issues starting: %w", err)
			continue
		}
		node.cmd = runCmd
		log.Infof("execution node started after %d retries", retries)

		if node.index == 0 {
			client, err := rpc.DialHTTP(e2e.TestParams.ExecutionNodeRPCURL(e2e.ExecutionNodeComponentOffset).String())
			if err != nil {
				return fmt.Errorf("failed to connect to ipc: %w", err)
			}

			web3 := zondclient.NewClient(client)
			block, err := web3.BlockByNumber(ctx, nil)
			if err != nil {
				return err
			}

			e2e.TestParams.ELGenesisBlock = block
		}

		break
	}
	if retryErr != nil {
		return retryErr
	}

	// Mark node as ready.
	close(node.started)

	return node.cmd.Wait()
}

// Started checks whether zond node is started and ready to be queried.
func (node *ExecutionNode) Started() <-chan struct{} {
	return node.started
}

// Pause pauses the component and its underlying process.
func (node *ExecutionNode) Pause() error {
	return node.cmd.Process.Signal(syscall.SIGSTOP)
}

// Resume resumes the component and its underlying process.
func (node *ExecutionNode) Resume() error {
	return node.cmd.Process.Signal(syscall.SIGCONT)
}

// Stop kills the component and its underlying process.
func (node *ExecutionNode) Stop() error {
	return node.cmd.Process.Kill()
}

func (node *ExecutionNode) UnderlyingProcess() *os.Process {
	return node.cmd.Process
}
