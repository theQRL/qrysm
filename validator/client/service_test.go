package client

import (
	"context"
	"errors"
	"testing"
	"time"

	logTest "github.com/sirupsen/logrus/hooks/test"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	validatorHelpers "github.com/theQRL/qrysm/validator/helpers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var _ runtime.Service = (*ValidatorService)(nil)
var _ GenesisFetcher = (*ValidatorService)(nil)
var _ SyncChecker = (*ValidatorService)(nil)

func TestStop_CancelsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	vs := &ValidatorService{
		ctx:    ctx,
		cancel: cancel,
	}

	assert.NoError(t, vs.Stop())

	select {
	case <-time.After(1 * time.Second):
		t.Error("Context not canceled within 1s")
	case <-vs.ctx.Done():
	}
}

func TestNew_Insecure(t *testing.T) {
	hook := logTest.NewGlobal()
	_, err := NewValidatorService(context.Background(), &Config{})
	require.NoError(t, err)
	require.LogsContain(t, hook, "You are using an insecure gRPC connection")
}

func TestStatus_NoConnectionError(t *testing.T) {
	validatorService := &ValidatorService{}
	assert.ErrorContains(t, "no connection", validatorService.Status())
}

func TestStart_GrpcHeaders(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()
	for input, output := range map[string][]string{
		"should-break": {},
		"key=value":    {"key", "value"},
		"":             {},
		",":            {},
		"key=value,Authorization=Q=": {
			"key", "value", "Authorization", "Q=",
		},
		"Authorization=this is a valid value": {
			"Authorization", "this is a valid value",
		},
	} {
		cfg := &Config{GrpcHeadersFlag: input}
		validatorService, err := NewValidatorService(ctx, cfg)
		require.NoError(t, err)
		md, _ := metadata.FromOutgoingContext(validatorService.ctx)
		if input == "should-break" {
			require.LogsContain(t, hook, "Incorrect gRPC header flag format. Skipping should-break")
		} else if len(output) == 0 {
			require.DeepEqual(t, md, metadata.MD(nil))
		} else {
			require.DeepEqual(t, md, metadata.Pairs(output...))
		}
	}
}

type fakeHealthTracker struct {
	errs   []error
	calls  int
	closed bool
}

func (f *fakeHealthTracker) EnsureHealthy(context.Context) error {
	err := f.errs[0]
	f.errs = f.errs[1:]
	f.calls++
	return err
}

func (*fakeHealthTracker) CurrentHost() string {
	return "secondary:4000"
}

func (*fakeHealthTracker) Invoke(context.Context, string, interface{}, interface{}, ...grpc.CallOption) error {
	return nil
}

func (*fakeHealthTracker) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return &fakeClientStream{}, nil
}

func (f *fakeHealthTracker) Close() error {
	f.closed = true
	return nil
}

func TestWaitForRunnerRecovery_UsesHealthyConnectionProvider(t *testing.T) {
	oldBackoff := backOffPeriod
	backOffPeriod = 0
	defer func() {
		backOffPeriod = oldBackoff
	}()

	tracker := &fakeHealthTracker{
		errs: []error{
			errors.New("primary unavailable"),
			nil,
		},
	}
	validatorService := &ValidatorService{
		conn: validatorHelpers.NewNodeConnection(tracker, "", 0, tracker.Close),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, validatorService.waitForRunnerRecovery(ctx))
	assert.Equal(t, 2, tracker.calls)
}

func TestWaitForRunnerRecovery_RequiresNonSyncingNodeWithoutHealthTracker(t *testing.T) {
	oldBackoff := backOffPeriod
	backOffPeriod = 0
	defer func() {
		backOffPeriod = oldBackoff
	}()

	callCount := 0
	conn := &fakeClientConn{
		invokeFn: func(_ string, reply interface{}) error {
			callCount++
			syncStatus, ok := reply.(*qrysmpb.SyncStatus)
			require.Equal(t, true, ok)
			syncStatus.Syncing = callCount == 1
			return nil
		},
	}
	validatorService := &ValidatorService{
		conn: validatorHelpers.NewNodeConnection(conn, "", 0, conn.Close),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, validatorService.waitForRunnerRecovery(ctx))
	assert.Equal(t, 2, callCount)
}
