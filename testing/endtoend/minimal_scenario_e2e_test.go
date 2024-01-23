package endtoend

import (
	"testing"

	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/endtoend/types"
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

// NOTE(rgeraldes24)
/*
func TestEndToEnd_ScenarioRun_EEOffline(t *testing.T) {
	t.Skip("TODO(#10242) Qrysm is current unable to handle an offline e2e")
	runner := e2eMinimal(t, version.Capella)

	runner.config.Evaluators = scenarioEvals()
	runner.config.EvalInterceptor = runner.eeOffline
	runner.scenarioRunner()
}
*/
