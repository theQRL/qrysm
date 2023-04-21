package ssz_static

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/altair/ssz_static"
)

func TestMainnet_Altair_SSZStatic(t *testing.T) {
	ssz_static.RunSSZStaticTests(t, "mainnet")
}
