package ssz_static

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/bellatrix/ssz_static"
)

func TestMainnet_Bellatrix_SSZStatic(t *testing.T) {
	ssz_static.RunSSZStaticTests(t, "mainnet")
}
