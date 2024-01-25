package endtoend

import (
	"testing"

	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/endtoend/types"
)

func TestEndToEnd_MultiScenarioRun(t *testing.T) {
	runner := e2eMainnet(t, false, types.StartAt(version.Capella, params.E2EMainnetTestConfig()), types.WithEpochs(24))
	runner.config.Evaluators = scenarioEvals()
	runner.config.EvalInterceptor = runner.multiScenario
	runner.scenarioRunner()
}
