package sync

import (
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/cmd/beacon-chain/flags"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)

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
