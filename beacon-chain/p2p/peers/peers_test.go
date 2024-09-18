package peers_test

import (
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/cmd/beacon-chain/flags"
	"github.com/theQRL/qrysm/config/features"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)

	resetCfg := features.InitWithReset(&features.Flags{
		EnablePeerScorer: true,
	})
	defer resetCfg()

	resetFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{
		BlockBatchLimit:            64,
		BlockBatchLimitBurstFactor: 10,
	})
	defer func() {
		flags.Init(resetFlags)
	}()
	m.Run()
}
