package logcapitalization_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/theQRL/qrysm/build/bazel"
	"github.com/theQRL/qrysm/tools/analyzers/logcapitalization"
)

func init() {
	if bazel.BuiltWithBazel() {
		bazel.SetGoEnv()
	}
}

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.RunWithSuggestedFixes(t, testdata, logcapitalization.Analyzer, "a")
}
