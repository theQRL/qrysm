package ssz_static

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/deneb/ssz_static"
)

func TestMainnet_Deneb_SSZStatic(t *testing.T) {
	ssz_static.RunSSZStaticTests(t, "mainnet")
}
