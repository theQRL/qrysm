package ssz_static

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/capella/ssz_static"
)

func TestMinimal_Capella_SSZStatic(t *testing.T) {
	ssz_static.RunSSZStaticTests(t, "minimal")
}
