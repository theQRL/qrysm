package endtoend

import (
	"testing"

	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/endtoend/types"
)

func TestEndToEnd_MultiScenarioRun_Minimal(t *testing.T) {
	runner := e2eMinimal(t, version.Capella, types.WithEpochs(24))

	runner.config.Evaluators = scenarioEvals()
	runner.config.EvalInterceptor = runner.multiScenario
	runner.scenarioRunner()
}

/*
func TestEndToEnd_MinimalConfig_Web3Signer(t *testing.T) {
	e2eMinimal(t, version.Capella, types.WithRemoteSigner()).run()
}
*/

func TestEndToEnd_MinimalConfig_ValidatorRESTApi(t *testing.T) {
	e2eMinimal(t, version.Capella /*types.WithCheckpointSync(),*/, types.WithValidatorRESTApi()).run()
}
