package zond

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/theQRL/go-zond/accounts/abi/bind"
	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/go-zond/rpc"
	"github.com/theQRL/go-zond/zondclient"
	"github.com/theQRL/qrysm/v4/config/params"
	contracts "github.com/theQRL/qrysm/v4/contracts/deposit"
	"github.com/theQRL/qrysm/v4/io/file"
	"github.com/theQRL/qrysm/v4/runtime/interop"
	"github.com/theQRL/qrysm/v4/testing/endtoend/helpers"
	e2e "github.com/theQRL/qrysm/v4/testing/endtoend/params"
	e2etypes "github.com/theQRL/qrysm/v4/testing/endtoend/types"
)

// Miner represents a zond node which mines blocks.
type Miner struct {
	e2etypes.ComponentRunner
	started      chan struct{}
	bootstrapEnr string
	enr          string
	cmd          *exec.Cmd
}

// NewMiner creates and returns a zond node miner.
func NewMiner() *Miner {
	return &Miner{
		started: make(chan struct{}, 1),
	}
}

// ENR returns the miner's enode.
func (m *Miner) ENR() string {
	return m.enr
}

// SetBootstrapENR sets the bootstrap record.
func (m *Miner) SetBootstrapENR(bootstrapEnr string) {
	m.bootstrapEnr = bootstrapEnr
}

func (*Miner) DataDir(sub ...string) string {
	parts := append([]string{e2e.TestParams.TestPath, "zonddata/miner"}, sub...)
	return path.Join(parts...)
}

func (*Miner) Password() string {
	return KeystorePassword
}

func (m *Miner) initDataDir() error {
	zondPath := m.DataDir()
	// Clear out potentially existing dir to prevent issues.
	if _, err := os.Stat(zondPath); !os.IsNotExist(err) {
		if err = os.RemoveAll(zondPath); err != nil {
			return err
		}
	}
	return nil
}

func (m *Miner) initAttempt(ctx context.Context, attempt int) (*os.File, error) {
	if err := m.initDataDir(); err != nil {
		return nil, err
	}

	// find gzond so we can run it.
	binaryPath, found := bazel.FindBinary("cmd/gzond", "gzond")
	if !found {
		return nil, errors.New("go-zond binary not found")
	}

	gzondJsonPath := path.Join(path.Dir(binaryPath), "genesis.json")
	gen := interop.GzondTestnetGenesis(e2e.TestParams.ZondGenesisTime, params.BeaconConfig())
	log.Infof("zond miner genesis timestamp=%d", e2e.TestParams.ZondGenesisTime)
	b, err := json.Marshal(gen)
	if err != nil {
		return nil, err
	}
	if err := file.WriteFile(gzondJsonPath, b); err != nil {
		return nil, err
	}

	// write the same thing to the logs dir for inspection
	gzondJsonLogPath := e2e.TestParams.Logfile("genesis.json")
	if err := file.WriteFile(gzondJsonLogPath, b); err != nil {
		return nil, err
	}

	initCmd := exec.CommandContext(ctx, binaryPath, "init", fmt.Sprintf("--datadir=%s", m.DataDir()), gzondJsonLogPath) // #nosec G204 -- Safe

	// redirect stderr to a log file
	initFile, err := helpers.DeleteAndCreatePath(e2e.TestParams.Logfile("zond-init_miner.log"))
	if err != nil {
		return nil, err
	}
	initCmd.Stderr = initFile

	// run init command and wait until it exits. this will initialize the gzond node (required before starting).
	if err = initCmd.Start(); err != nil {
		return nil, err
	}
	if err = initCmd.Wait(); err != nil {
		return nil, err
	}

	pwFile := m.DataDir("keystore", minerPasswordFile)
	args := []string{
		"--nat=none", // disable nat traversal in e2e, it is failure prone and not needed
		fmt.Sprintf("--datadir=%s", m.DataDir()),
		fmt.Sprintf("--http.port=%d", e2e.TestParams.Ports.ZondRPCPort),
		fmt.Sprintf("--ws.port=%d", e2e.TestParams.Ports.ZondWSPort),
		fmt.Sprintf("--authrpc.port=%d", e2e.TestParams.Ports.ZondAuthRPCPort),
		fmt.Sprintf("--bootnodes=%s", m.bootstrapEnr),
		fmt.Sprintf("--port=%d", e2e.TestParams.Ports.ZondPort),
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
	}

	keystorePath, err := e2e.TestParams.Paths.MinerKeyPath()
	if err != nil {
		return nil, err
	}
	if err = file.CopyFile(keystorePath, m.DataDir("keystore", minerFile)); err != nil {
		return nil, errors.Wrapf(err, "error copying %s to %s", keystorePath, m.DataDir("keystore", minerFile))
	}
	err = file.WriteFile(pwFile, []byte(KeystorePassword))
	if err != nil {
		return nil, err
	}

	runCmd := exec.CommandContext(ctx, binaryPath, args...) // #nosec G204 -- Safe
	// redirect miner stderr to a log file
	minerLog, err := helpers.DeleteAndCreatePath(e2e.TestParams.Logfile("zond_miner.log"))
	if err != nil {
		return nil, err
	}
	runCmd.Stderr = minerLog
	log.Infof("Starting zond miner, attempt %d, with flags: %s", attempt, strings.Join(args[2:], " "))
	if err = runCmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start zond chain: %w", err)
	}
	if err = helpers.WaitForTextInFile(minerLog, "Started P2P networking"); err != nil {
		kerr := runCmd.Process.Kill()
		if kerr != nil {
			log.WithError(kerr).Error("error sending kill to failed miner command process")
		}
		return nil, fmt.Errorf("P2P log not found, this means the zond chain had issues starting: %w", err)
	}
	m.cmd = runCmd
	return minerLog, nil
}

// Start runs a mining zond node.
// The miner is responsible for moving the zond chain forward and for deploying the deposit contract.
func (m *Miner) Start(ctx context.Context) error {
	// give the miner start a couple of tries, since the p2p networking check is flaky
	var retryErr error
	var minerLog *os.File
	for attempt := 0; attempt < 3; attempt++ {
		minerLog, retryErr = m.initAttempt(ctx, attempt)
		if retryErr == nil {
			log.Infof("miner started after %d retries", attempt)
			break
		}
	}
	if retryErr != nil {
		return retryErr
	}

	enode, err := enodeFromLogFile(minerLog.Name())
	if err != nil {
		return err
	}
	enode = "enode://" + enode + "@127.0.0.1:" + fmt.Sprintf("%d", e2e.TestParams.Ports.ZondPort)
	m.enr = enode
	log.Infof("Communicated enode. Enode is %s", enode)

	// Connect to the started gzond dev chain.
	client, err := rpc.DialHTTP(e2e.TestParams.ZondRPCURL(e2e.MinerComponentOffset).String())
	if err != nil {
		return fmt.Errorf("failed to connect to ipc: %w", err)
	}
	web3 := zondclient.NewClient(client)
	block, err := web3.BlockByNumber(ctx, nil)
	if err != nil {
		return err
	}
	log.Infof("genesis block timestamp=%d", block.Time())
	zondBlockHash := block.Hash()
	e2e.TestParams.ZondGenesisBlock = block
	log.Infof("miner says genesis block root=%#x", zondBlockHash)
	cAddr := common.HexToAddress(params.BeaconConfig().DepositContractAddress)
	code, err := web3.CodeAt(ctx, cAddr, nil)
	if err != nil {
		return err
	}
	log.Infof("contract code size = %d", len(code))
	depositContractCaller, err := contracts.NewDepositContractCaller(cAddr, web3)
	if err != nil {
		return err
	}
	dCount, err := depositContractCaller.GetDepositCount(&bind.CallOpts{})
	if err != nil {
		log.Error("failed to call get_deposit_count method of deposit contract")
		return err
	}
	log.Infof("deposit contract count=%d", dCount)

	// Mark node as ready.
	close(m.started)
	return m.cmd.Wait()
}

// Started checks whether zond node is started and ready to be queried.
func (m *Miner) Started() <-chan struct{} {
	return m.started
}

// Pause pauses the component and its underlying process.
func (m *Miner) Pause() error {
	return m.cmd.Process.Signal(syscall.SIGSTOP)
}

// Resume resumes the component and its underlying process.
func (m *Miner) Resume() error {
	return m.cmd.Process.Signal(syscall.SIGCONT)
}

// Stop kills the component and its underlying process.
func (m *Miner) Stop() error {
	return m.cmd.Process.Kill()
}

func enodeFromLogFile(name string) (string, error) {
	byteContent, err := os.ReadFile(name) // #nosec G304
	if err != nil {
		return "", err
	}
	contents := string(byteContent)

	searchText := "self=enode://"
	startIdx := strings.Index(contents, searchText)
	if startIdx == -1 {
		return "", fmt.Errorf("did not find ENR text in %s", contents)
	}
	startIdx += len(searchText)
	endIdx := strings.Index(contents[startIdx:], "@")
	if endIdx == -1 {
		return "", fmt.Errorf("did not find ENR text in %s", contents)
	}
	enode := contents[startIdx : startIdx+endIdx]
	return strings.TrimPrefix(enode, "-"), nil
}
