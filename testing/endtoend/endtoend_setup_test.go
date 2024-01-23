package endtoend

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/theQRL/qrysm/v4/config/params"
	ev "github.com/theQRL/qrysm/v4/testing/endtoend/evaluators"
	e2eParams "github.com/theQRL/qrysm/v4/testing/endtoend/params"
	"github.com/theQRL/qrysm/v4/testing/endtoend/types"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func e2eMinimal(t *testing.T, v int, cfgo ...types.E2EConfigOpt) *testRunner {
	params.SetupTestConfigCleanup(t)
	require.NoError(t, params.SetActive(types.StartAt(v, params.E2ETestConfig())))
	require.NoError(t, e2eParams.Init(t, e2eParams.StandardBeaconCount))

	// Run for 12 epochs if not in long-running to confirm long-running has no issues.
	var err error
	epochsToRun := 12
	epochStr, longRunning := os.LookupEnv("E2E_EPOCHS")
	if longRunning {
		epochsToRun, err = strconv.Atoi(epochStr)
		require.NoError(t, err)
	}
	seed := 0
	seedStr, isValid := os.LookupEnv("E2E_SEED")
	if isValid {
		seed, err = strconv.Atoi(seedStr)
		require.NoError(t, err)
	}
	tracingPort := e2eParams.TestParams.Ports.JaegerTracingPort
	tracingEndpoint := fmt.Sprintf("127.0.0.1:%d", tracingPort)
	evals := []types.Evaluator{
		ev.PeersConnect,
		ev.HealthzCheck,
		ev.MetricsCheck,
		ev.ValidatorsAreActive,
		ev.ValidatorsParticipatingAtEpoch(2),
		ev.FinalizationOccurs(3),
		ev.VerifyBlockGraffiti,
		ev.PeersCheck,
		ev.ProposeVoluntaryExit,
		ev.ValidatorsHaveExited,
		ev.SubmitWithdrawal,
		ev.ValidatorsHaveWithdrawn,
		ev.ProcessesDepositsInBlocks,
		ev.ActivatesDepositedValidators,
		ev.DepositedValidatorsAreActive,
		ev.ValidatorsVoteWithTheMajority,
		ev.ColdStateCheckpoint,
		// ev.DenebForkTransition, // TODO(12750): Enable this when gzond main branch's engine API support.
		ev.APIMiddlewareVerifyIntegrity,
		ev.APIGatewayV1Alpha1VerifyIntegrity,
		ev.FinishedSyncing,
		ev.AllNodesHaveSameHead,
		ev.ValidatorSyncParticipation,
		ev.FeeRecipientIsPresent,
		//ev.TransactionsPresent, TODO: Re-enable Transaction evaluator once it tx pool issues are fixed.
	}
	testConfig := &types.E2EConfig{
		BeaconFlags: []string{
			fmt.Sprintf("--slots-per-archive-point=%d", params.BeaconConfig().SlotsPerEpoch*16),
			fmt.Sprintf("--tracing-endpoint=http://%s", tracingEndpoint),
			"--enable-tracing",
			"--trace-sample-fraction=1.0",
		},
		ValidatorFlags:      []string{},
		EpochsToRun:         uint64(epochsToRun),
		TestSync:            true,
		TestFeature:         true,
		TestDeposits:        true,
		UseQrysmShValidator: false,
		UsePprof:            !longRunning,
		TracingSinkEndpoint: tracingEndpoint,
		Evaluators:          evals,
		EvalInterceptor:     defaultInterceptor,
		Seed:                int64(seed),
	}
	for _, o := range cfgo {
		o(testConfig)
	}
	if testConfig.UseBuilder {
		testConfig.Evaluators = append(testConfig.Evaluators, ev.BuilderIsActive)
	}

	return newTestRunner(t, testConfig)
}

func e2eMainnet(t *testing.T, useQrysmSh bool, cfg *params.BeaconChainConfig, cfgo ...types.E2EConfigOpt) *testRunner {
	params.SetupTestConfigCleanup(t)
	require.NoError(t, params.SetActive(cfg))
	require.NoError(t, e2eParams.Init(t, e2eParams.StandardBeaconCount))
	// Run for 10 epochs if not in long-running to confirm long-running has no issues.
	var err error
	epochsToRun := 12
	epochStr, longRunning := os.LookupEnv("E2E_EPOCHS")
	if longRunning {
		epochsToRun, err = strconv.Atoi(epochStr)
		require.NoError(t, err)
	}
	seed := 0
	seedStr, isValid := os.LookupEnv("E2E_SEED")
	if isValid {
		seed, err = strconv.Atoi(seedStr)
		require.NoError(t, err)
	}
	tracingPort := e2eParams.TestParams.Ports.JaegerTracingPort
	tracingEndpoint := fmt.Sprintf("127.0.0.1:%d", tracingPort)
	evals := []types.Evaluator{
		ev.PeersConnect,
		ev.HealthzCheck,
		ev.MetricsCheck,
		ev.ValidatorsParticipatingAtEpoch(2),
		ev.FinalizationOccurs(3),
		ev.ProposeVoluntaryExit,
		ev.ValidatorsHaveExited,
		ev.SubmitWithdrawal,
		ev.ValidatorsHaveWithdrawn,
		ev.DepositedValidatorsAreActive,
		ev.ColdStateCheckpoint,
		// ev.DenebForkTransition, // TODO(12750): Enable this when gzond main branch's engine API support.
		ev.APIMiddlewareVerifyIntegrity,
		ev.APIGatewayV1Alpha1VerifyIntegrity,
		ev.FinishedSyncing,
		ev.AllNodesHaveSameHead,
		ev.FeeRecipientIsPresent,
		//ev.TransactionsPresent, TODO: Re-enable Transaction evaluator once it tx pool issues are fixed.
	}
	testConfig := &types.E2EConfig{
		BeaconFlags: []string{
			fmt.Sprintf("--slots-per-archive-point=%d", params.BeaconConfig().SlotsPerEpoch*16),
			fmt.Sprintf("--tracing-endpoint=http://%s", tracingEndpoint),
			"--enable-tracing",
			"--trace-sample-fraction=1.0",
		},
		ValidatorFlags:      []string{},
		EpochsToRun:         uint64(epochsToRun),
		TestSync:            true,
		TestFeature:         true,
		TestDeposits:        true,
		UseFixedPeerIDs:     true,
		UseQrysmShValidator: useQrysmSh,
		UsePprof:            !longRunning,
		TracingSinkEndpoint: tracingEndpoint,
		Evaluators:          evals,
		EvalInterceptor:     defaultInterceptor,
		Seed:                int64(seed),
	}
	for _, o := range cfgo {
		o(testConfig)
	}

	if testConfig.UseBuilder {
		testConfig.Evaluators = append(testConfig.Evaluators, ev.BuilderIsActive)
	}
	return newTestRunner(t, testConfig)
}

func scenarioEvals() []types.Evaluator {
	return []types.Evaluator{
		ev.PeersConnect,
		ev.HealthzCheck,
		ev.MetricsCheck,
		ev.ValidatorsParticipatingAtEpoch(2),
		ev.FinalizationOccurs(3),
		ev.VerifyBlockGraffiti,
		ev.ProposeVoluntaryExit,
		ev.ValidatorsHaveExited,
		ev.ColdStateCheckpoint,
		// ev.DenebForkTransition, // TODO(12750): Enable this when gzond main branch's engine API support.
		ev.APIMiddlewareVerifyIntegrity,
		ev.APIGatewayV1Alpha1VerifyIntegrity,
		ev.FinishedSyncing,
		ev.AllNodesHaveSameHead,
		ev.ValidatorSyncParticipation,
	}
}
