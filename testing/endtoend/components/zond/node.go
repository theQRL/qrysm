package zond

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
	log "github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/io/file"
	"github.com/theQRL/qrysm/v4/runtime/interop"
	"github.com/theQRL/qrysm/v4/testing/endtoend/helpers"
	e2e "github.com/theQRL/qrysm/v4/testing/endtoend/params"
	e2etypes "github.com/theQRL/qrysm/v4/testing/endtoend/types"
)

// Node represents a zond node.
type Node struct {
	e2etypes.ComponentRunner
	started chan struct{}
	index   int
	enr     string
	cmd     *exec.Cmd
}

// NewNode creates and returns a zond node.
func NewNode(index int, enr string) *Node {
	return &Node{
		started: make(chan struct{}, 1),
		index:   index,
		enr:     enr,
	}
}

// Start runs a non-mining zond node.
// To connect to a miner and start working properly, this node should be a part of a NodeSet.
func (node *Node) Start(ctx context.Context) error {
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

	gen := interop.GethTestnetGenesis(e2e.TestParams.ZondGenesisTime, params.BeaconConfig())
	b, err := json.Marshal(gen)
	if err != nil {
		return err
	}

	if err := file.WriteFile(gzondJsonPath, b); err != nil {
		return err
	}
	copyPath := path.Join(e2e.TestParams.LogPath, "zond-genesis.json")
	if err := file.WriteFile(copyPath, b); err != nil {
		return err
	}

	initCmd := exec.CommandContext(ctx, binaryPath, "init", fmt.Sprintf("--datadir=%s", zondPath), gzondJsonPath) // #nosec G204 -- Safe
	initFile, err := helpers.DeleteAndCreateFile(e2e.TestParams.LogPath, "zond-init_"+strconv.Itoa(node.index)+".log")
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
		fmt.Sprintf("--http.port=%d", e2e.TestParams.Ports.ZondRPCPort+node.index),
		fmt.Sprintf("--ws.port=%d", e2e.TestParams.Ports.ZondWSPort+node.index),
		fmt.Sprintf("--authrpc.port=%d", e2e.TestParams.Ports.ZondAuthRPCPort+node.index),
		fmt.Sprintf("--bootnodes=%s", node.enr),
		fmt.Sprintf("--port=%d", e2e.TestParams.Ports.ZondPort+node.index),
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
		log.Infof("Starting zond node %d, attempt %d with flags: %s", node.index, retries, strings.Join(args[2:], " "))
		runCmd := exec.CommandContext(ctx, binaryPath, args...) // #nosec G204 -- Safe
		errLog, err := os.Create(path.Join(e2e.TestParams.LogPath, "zond_"+strconv.Itoa(node.index)+".log"))
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
		log.Infof("zond node started after %d retries", retries)
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
func (node *Node) Started() <-chan struct{} {
	return node.started
}

// Pause pauses the component and its underlying process.
func (node *Node) Pause() error {
	return node.cmd.Process.Signal(syscall.SIGSTOP)
}

// Resume resumes the component and its underlying process.
func (node *Node) Resume() error {
	return node.cmd.Process.Signal(syscall.SIGCONT)
}

// Stop kills the component and its underlying process.
func (node *Node) Stop() error {
	return node.cmd.Process.Kill()
}

func (node *Node) UnderlyingProcess() *os.Process {
	return node.cmd.Process
}
