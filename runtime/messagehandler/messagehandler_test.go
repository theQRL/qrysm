package messagehandler_test

import (
	"context"
	"testing"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/v4/runtime/messagehandler"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestSafelyHandleMessage(t *testing.T) {
	hook := logTest.NewGlobal()

	messagehandler.SafelyHandleMessage(context.Background(), func(_ context.Context, _ *pubsub.Message) error {
		panic("bad!")
		return nil
	}, &pubsub.Message{})

	require.LogsContain(t, hook, "Panicked when handling p2p message!")
}

func TestSafelyHandleMessage_NoData(t *testing.T) {
	hook := logTest.NewGlobal()

	messagehandler.SafelyHandleMessage(context.Background(), func(_ context.Context, _ *pubsub.Message) error {
		panic("bad!")
		return nil
	}, nil)

	entry := hook.LastEntry()
	if entry.Data["msg"] != "message contains no data" {
		t.Errorf("Message logged was not what was expected: %s", entry.Data["msg"])
	}
}
