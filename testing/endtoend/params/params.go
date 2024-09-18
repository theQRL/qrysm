// Package params defines all custom parameter configurations
// for running end to end tests.
package params

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/theQRL/go-zond/core/types"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/theQRL/qrysm/io/file"
)

// params struct defines the parameters needed for running E2E tests to properly handle test sharding.
type params struct {
	TestPath               string
	LogPath                string
	TestShardIndex         int
	BeaconNodeCount        int
	Ports                  *ports
	Paths                  *paths
	ELGenesisBlock         *types.Block
	ELGenesisTime          uint64
	StartTime              time.Time
	CLGenesisTime          uint64
	NumberOfExecutionCreds uint64
}

type ports struct {
	BootNodePort                  int
	BootNodeMetricsPort           int
	GzondExecutionNodePort        int
	GzondExecutionNodeRPCPort     int
	GzondExecutionNodeAuthRPCPort int
	GzondExecutionNodeWSPort      int
	ProxyPort                     int
	QrysmBeaconNodeRPCPort        int
	QrysmBeaconNodeUDPPort        int
	QrysmBeaconNodeTCPPort        int
	QrysmBeaconNodeGatewayPort    int
	QrysmBeaconNodeMetricsPort    int
	QrysmBeaconNodePprofPort      int
	ValidatorMetricsPort          int
	ValidatorGatewayPort          int
	JaegerTracingPort             int
}

type paths struct{}

// ZondStaticFile abstracts the location of the zond static file folder in the e2e directory, so that
// a relative path can be used.
// The relative path is specified as a variadic slice of path parts, in the same way as path.Join.
func (*paths) ZondStaticFile(rel ...string) string {
	parts := append([]string{StaticFilesPath}, rel...)
	return path.Join(parts...)
}

// ZondRunfile returns the full path to a file in the zond static directory, within bazel's run context.
// The relative path is specified as a variadic slice of path parts, in the same style as path.Join.
func (p *paths) ZondRunfile(rel ...string) (string, error) {
	return bazel.Runfile(p.ZondStaticFile(rel...))
}

// TestKeyPath returns the full path to the file containing the test cryptographic keys.
func (p *paths) TestKeyPath() (string, error) {
	return p.ZondRunfile(keyFilename)
}

// TestParams is the globally accessible var for getting config elements.
var TestParams *params

// Logfile gives the full path to a file in the bazel test environment log directory.
// The relative path is specified as a variadic slice of path parts, in the same style as path.Join.
func (p *params) Logfile(rel ...string) string {
	return path.Join(append([]string{p.LogPath}, rel...)...)
}

// ExecutionNodeRPCURL gives the full url to use to connect to the given execution client's RPC endpoint.
// The `index` param corresponds to the `index` field of the `zond.ExecutionNode` e2e component.
// These are off by one compared to corresponding beacon nodes, because the miner is assigned index 0.
// eg instance the index of the EL instance associated with beacon node index `0` would typically be `1`.
func (p *params) ExecutionNodeRPCURL(index int) *url.URL {
	return &url.URL{
		Scheme: baseELScheme,
		Host:   net.JoinHostPort(baseELHost, fmt.Sprintf("%d", p.Ports.GzondExecutionNodeRPCPort+index)),
	}
}

// BootNodeLogFileName is the file name used for the beacon chain node logs.
var BootNodeLogFileName = "bootnode.log"

// TracingRequestSinkFileName is the file name for writing raw trace requests.
var TracingRequestSinkFileName = "tracing-http-requests.log.gz"

// BeaconNodeLogFileName is the file name used for the beacon chain node logs.
var BeaconNodeLogFileName = "beacon-%d.log"

// ValidatorLogFileName is the file name used for the validator client logs.
var ValidatorLogFileName = "vals-%d.log"

// StandardBeaconCount is a global constant for the count of beacon nodes of standard E2E tests.
var StandardBeaconCount = 2

// DepositCount is the number of deposits the E2E runner should make to evaluate post-genesis deposit processing.
var DepositCount = uint64(64)

// PregenesisExecCreds is the number of withdrawal credentials of genesis validators which use an execution address.
var PregenesisExecCreds = uint64(8)

// NumOfExecEngineTxs is the number of transaction sent to the execution engine.
var NumOfExecEngineTxs = uint64(200)

// ExpectedExecEngineTxsThreshold is the portion of execution engine transactions we expect to find in blocks.
var ExpectedExecEngineTxsThreshold = 0.5

// Base port values.
const (
	portSpan = 50

	BootNodePort        = 2150
	BootNodeMetricsPort = BootNodePort + portSpan

	GzondExecutionNodePort        = 3150
	GzondExecutionNodeRPCPort     = GzondExecutionNodePort + portSpan
	GzondExecutionNodeWSPort      = GzondExecutionNodePort + 2*portSpan
	GzondExecutionNodeAuthRPCPort = GzondExecutionNodePort + 3*portSpan
	ExecutionNodeProxyPort        = GzondExecutionNodePort + 4*portSpan

	QrysmBeaconNodeRPCPort     = 4150
	QrysmBeaconNodeUDPPort     = QrysmBeaconNodeRPCPort + portSpan
	QrysmBeaconNodeTCPPort     = QrysmBeaconNodeRPCPort + 2*portSpan
	QrysmBeaconNodeGatewayPort = QrysmBeaconNodeRPCPort + 3*portSpan
	QrysmBeaconNodeMetricsPort = QrysmBeaconNodeRPCPort + 4*portSpan
	QrysmBeaconNodePprofPort   = QrysmBeaconNodeRPCPort + 5*portSpan

	ValidatorGatewayPort = 6150
	ValidatorMetricsPort = ValidatorGatewayPort + portSpan

	JaegerTracingPort = 9150

	StartupBufferSecs = 15
)

func logDir() string {
	wTime := func(p string) string {
		return path.Join(p, time.Now().Format("20060102/150405"))
	}
	path, ok := os.LookupEnv("E2E_LOG_PATH")
	if ok {
		return wTime(path)
	}
	path, _ = os.LookupEnv("TEST_UNDECLARED_OUTPUTS_DIR")
	return wTime(path)
}

// Init initializes the E2E config, properly handling test sharding.
func Init(t *testing.T, beaconNodeCount int) error {
	d := logDir()
	if d == "" {
		return errors.New("unable to determine log directory, no value for E2E_LOG_PATH or TEST_UNDECLARED_OUTPUTS_DIR")
	}
	logPath := path.Join(d, t.Name())
	if err := file.MkdirAll(logPath); err != nil {
		return err
	}
	testPath := bazel.TestTmpDir()
	testTotalShardsStr, ok := os.LookupEnv("TEST_TOTAL_SHARDS")
	if !ok {
		testTotalShardsStr = "1"
	}
	testTotalShards, err := strconv.Atoi(testTotalShardsStr)
	if err != nil {
		return err
	}
	testShardIndexStr, ok := os.LookupEnv("TEST_SHARD_INDEX")
	if !ok {
		testShardIndexStr = "0"
	}
	testShardIndex, err := strconv.Atoi(testShardIndexStr)
	if err != nil {
		return err
	}

	var existingRegistrations []int
	testPorts := &ports{}
	err = initializeStandardPorts(testTotalShards, testShardIndex, testPorts, &existingRegistrations)
	if err != nil {
		return err
	}

	genTime := uint64(time.Now().Unix()) + StartupBufferSecs
	TestParams = &params{
		TestPath:               filepath.Join(testPath, fmt.Sprintf("shard-%d", testShardIndex)),
		LogPath:                logPath,
		TestShardIndex:         testShardIndex,
		BeaconNodeCount:        beaconNodeCount,
		Ports:                  testPorts,
		CLGenesisTime:          genTime,
		ELGenesisTime:          genTime,
		NumberOfExecutionCreds: PregenesisExecCreds,
	}
	return nil
}

// port returns a safe port number based on the seed and shard data.
func port(seed, shardCount, shardIndex int, existingRegistrations *[]int) (int, error) {
	portToRegister := seed + portSpan/shardCount*shardIndex
	for _, p := range *existingRegistrations {
		if portToRegister >= p && portToRegister <= p+(portSpan/shardCount)-1 {
			return 0, fmt.Errorf("port %d overlaps with already registered port %d", seed, p)
		}
	}
	*existingRegistrations = append(*existingRegistrations, portToRegister)

	// Calculation example: 3 shards, seed 2000, port span 50.
	// Shard 0: 2000 + (50 / 3 * 0) = 2000 (we can safely use ports 2000-2015)
	// Shard 1: 2000 + (50 / 3 * 1) = 2016 (we can safely use ports 2016-2031)
	// Shard 2: 2000 + (50 / 3 * 2) = 2032 (we can safely use ports 2032-2047, and in reality 2032-2049)
	return portToRegister, nil
}

func initializeStandardPorts(shardCount, shardIndex int, ports *ports, existingRegistrations *[]int) error {
	bootnodePort, err := port(BootNodePort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	bootnodeMetricsPort, err := port(BootNodeMetricsPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	executionNodePort, err := port(GzondExecutionNodePort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	executionNodeRPCPort, err := port(GzondExecutionNodeRPCPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	executionNodeWSPort, err := port(GzondExecutionNodeWSPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	executionNodeAuthPort, err := port(GzondExecutionNodeAuthRPCPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	executionNodeProxyPort, err := port(ExecutionNodeProxyPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	beaconNodeRPCPort, err := port(QrysmBeaconNodeRPCPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	beaconNodeUDPPort, err := port(QrysmBeaconNodeUDPPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	beaconNodeTCPPort, err := port(QrysmBeaconNodeTCPPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	beaconNodeGatewayPort, err := port(QrysmBeaconNodeGatewayPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	beaconNodeMetricsPort, err := port(QrysmBeaconNodeMetricsPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	beaconNodePprofPort, err := port(QrysmBeaconNodePprofPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	validatorGatewayPort, err := port(ValidatorGatewayPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	validatorMetricsPort, err := port(ValidatorMetricsPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	jaegerTracingPort, err := port(JaegerTracingPort, shardCount, shardIndex, existingRegistrations)
	if err != nil {
		return err
	}
	ports.BootNodePort = bootnodePort
	ports.BootNodeMetricsPort = bootnodeMetricsPort
	ports.GzondExecutionNodePort = executionNodePort
	ports.GzondExecutionNodeRPCPort = executionNodeRPCPort
	ports.GzondExecutionNodeAuthRPCPort = executionNodeAuthPort
	ports.GzondExecutionNodeWSPort = executionNodeWSPort
	ports.ProxyPort = executionNodeProxyPort
	ports.QrysmBeaconNodeRPCPort = beaconNodeRPCPort
	ports.QrysmBeaconNodeUDPPort = beaconNodeUDPPort
	ports.QrysmBeaconNodeTCPPort = beaconNodeTCPPort
	ports.QrysmBeaconNodeGatewayPort = beaconNodeGatewayPort
	ports.QrysmBeaconNodeMetricsPort = beaconNodeMetricsPort
	ports.QrysmBeaconNodePprofPort = beaconNodePprofPort
	ports.ValidatorMetricsPort = validatorMetricsPort
	ports.ValidatorGatewayPort = validatorGatewayPort
	ports.JaegerTracingPort = jaegerTracingPort
	return nil
}
