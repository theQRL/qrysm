package bazel_test

import (
	"testing"

	"github.com/theQRL/qrysm/build/bazel"
)

func TestBuildWithBazel(t *testing.T) {
	if !bazel.BuiltWithBazel() {
		t.Error("not built with Bazel")
	}
}
