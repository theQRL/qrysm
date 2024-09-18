package endtoend

import (
	"testing"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/endtoend/types"
)

func TestEndToEnd_MultiScenarioRun(t *testing.T) {
	runner := e2eMainnet(t, false, types.StartAt(version.Capella, params.E2EMainnetTestConfig()), types.WithEpochs(24))
	runner.config.Evaluators = scenarioEvals()
	runner.config.EvalInterceptor = runner.multiScenario
	runner.scenarioRunner()
}
