package client

import (
	"context"
	"math/bits"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/async/event"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	validatorserviceconfig "github.com/theQRL/qrysm/config/validator/service"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/time/slots"
	"github.com/theQRL/qrysm/validator/client/iface"
	"github.com/theQRL/qrysm/validator/client/testutil"
	"go.opencensus.io/trace"
)

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestCancelledContext_CleansUpValidator(t *testing.T) {
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}}
	run(cancelledContext(), v)
	assert.Equal(t, true, v.DoneCalled, "Expected Done() to be called")
}

func TestCancelledContext_WaitsForChainStart(t *testing.T) {
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}}
	run(cancelledContext(), v)
	assert.Equal(t, 1, v.WaitForChainStartCalled, "Expected WaitForChainStart() to be called")
}

func TestRetry_On_ConnectionError(t *testing.T) {
	retry := 10
	v := &testutil.FakeValidator{
		Km:               &mockKeymanager{accountsChangedFeed: &event.Feed{}},
		RetryTillSuccess: retry,
	}
	originalBackOffPeriod := backOffPeriod
	defer func() {
		backOffPeriod = originalBackOffPeriod
	}()
	backOffPeriod = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	go run(ctx, v)
	// each step will fail (retry times)=10 this sleep times will wait more then
	// the time it takes for all steps to succeed before main loop.
	time.Sleep(time.Duration(retry*6) * backOffPeriod)
	cancel()
	assert.Equal(t, retry*2+1, v.WaitForChainStartCalled, "Expected WaitForChainStart() to be called")
	assert.Equal(t, retry+1, v.WaitForSyncCalled, "Expected WaitForSync() to be called")
	assert.Equal(t, 1, v.WaitForActivationCalled, "Expected WaitForActivation() to be called")
	assert.Equal(t, 0, v.CanonicalHeadSlotCalled, "Expected CanonicalHeadSlot() not to be called")
	assert.Equal(t, 1, v.ReceiveBlocksCalled, "Expected ReceiveBlocks() to be called once after startup succeeds")
}

func TestCancelledContext_WaitsForActivation(t *testing.T) {
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}}
	run(cancelledContext(), v)
	assert.Equal(t, 1, v.WaitForActivationCalled, "Expected WaitForActivation() to be called")
}

func TestRun_ReturnsKeymanagerError(t *testing.T) {
	v := &testutil.FakeValidator{
		KeymanagerFailures: 1,
		KeymanagerErr:      errors.New("boom"),
	}

	err := run(context.Background(), v)

	require.ErrorContains(t, "could not get keymanager", err)
	assert.Equal(t, true, v.DoneCalled, "Expected Done() to be called")
	assert.Equal(t, 1, v.KeymanagerCalled, "Expected Keymanager() to be called once")
}

func TestRunWithRecovery_RestartsAfterStartupError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	v := &testutil.FakeValidator{
		Km:                 &mockKeymanager{accountsChangedFeed: &event.Feed{}},
		KeymanagerFailures: 1,
		KeymanagerErr:      errors.New("boom"),
	}
	recoveryCalls := 0
	waitForRecovery := func(context.Context) error {
		recoveryCalls++
		return nil
	}

	go runWithRecovery(ctx, v, waitForRecovery)
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)

	assert.Equal(t, 2, v.WaitForChainStartCalled, "Expected the runner to restart after a startup failure")
	assert.Equal(t, 2, v.KeymanagerCalled, "Expected Keymanager() to be retried after a startup failure")
	assert.Equal(t, 1, recoveryCalls, "Expected one recovery wait between failed and successful runs")
}

func TestRunWithRecovery_RestartsAfterBlockStreamError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	v := &testutil.FakeValidator{
		Km:                            &mockKeymanager{accountsChangedFeed: &event.Feed{}},
		ReceiveBlocksRetryTillSuccess: 1,
	}
	recoveryCalls := 0
	waitForRecovery := func(context.Context) error {
		recoveryCalls++
		return nil
	}

	go runWithRecovery(ctx, v, waitForRecovery)
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)

	assert.Equal(t, 2, v.WaitForChainStartCalled, "Expected the runner to reinitialize after a block stream failure")
	assert.Equal(t, 2, v.ReceiveBlocksCalled, "Expected ReceiveBlocks() to be restarted through the recovery loop")
	assert.Equal(t, 1, recoveryCalls, "Expected one recovery wait between failed and successful runs")
}

type slotContextObserverValidator struct {
	*testutil.FakeValidator
	logCtxCh chan context.Context
}

func (v *slotContextObserverValidator) LogValidatorGainsAndLosses(ctx context.Context, _ primitives.Slot) error {
	select {
	case v.logCtxCh <- ctx:
	default:
	}
	return nil
}

func TestPerformRoles_CancelsSlotContextWhenComplete(t *testing.T) {
	slotCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Minute))
	defer cancel()

	observer := &slotContextObserverValidator{
		FakeValidator: &testutil.FakeValidator{},
		logCtxCh:      make(chan context.Context, 1),
	}

	_, span := trace.StartSpan(context.Background(), "test.performRoles")
	var wg sync.WaitGroup
	allRoles := map[[field_params.MLDSA87PubkeyLength]byte][]iface.ValidatorRole{
		{1}: {iface.RoleUnknown},
	}

	performRoles(slotCtx, allRoles, observer, primitives.Slot(1), &wg, span, cancel)

	var observedCtx context.Context
	select {
	case observedCtx = <-observer.logCtxCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for post-slot logging")
	}

	select {
	case <-observedCtx.Done():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for slot context cancellation")
	}
}

func TestRun_UsesCurrentSlotAfterActivation(t *testing.T) {
	genesisTime := uint64(time.Now().Add(time.Second).Unix())
	v := &testutil.FakeValidator{
		Km:       &mockKeymanager{accountsChangedFeed: &event.Feed{}},
		GenesisT: genesisTime,
	}
	require.NoError(t, v.SetProposerSettings(context.Background(), &validatorserviceconfig.ProposerSettings{}))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	run(ctx, v)

	expectedSlot := uint64(slots.CurrentSlot(genesisTime))
	assert.Equal(t, expectedSlot, v.UpdateDutiesArg1, "Expected initial UpdateDuties() to use the current slot")
	assert.Equal(t, expectedSlot, v.PushProposerSettingsArg1, "Expected initial PushProposerSettings() to use the current slot")
	assert.Equal(t, 0, v.CanonicalHeadSlotCalled, "Expected CanonicalHeadSlot() not to be called")
}

func TestUpdateDuties_NextSlot(t *testing.T) {
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}}
	ctx, cancel := context.WithCancel(context.Background())

	slot := primitives.Slot(55)
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	go func() {
		ticker <- slot

		cancel()
	}()

	run(ctx, v)

	require.Equal(t, true, v.UpdateDutiesCalled, "Expected UpdateAssignments(%d) to be called", slot)
	assert.Equal(t, uint64(slot), v.UpdateDutiesArg1, "UpdateAssignments was called with wrong argument")
}

func TestUpdateDuties_HandlesError(t *testing.T) {
	hook := logTest.NewGlobal()
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}}
	ctx, cancel := context.WithCancel(context.Background())

	slot := primitives.Slot(55)
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	go func() {
		ticker <- slot

		cancel()
	}()
	v.UpdateDutiesRet = errors.New("bad")

	run(ctx, v)

	require.LogsContain(t, hook, "Failed to update assignments")
}

func TestRoleAt_NextSlot(t *testing.T) {
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}}
	ctx, cancel := context.WithCancel(context.Background())

	slot := primitives.Slot(55)
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	go func() {
		ticker <- slot

		cancel()
	}()

	run(ctx, v)

	require.Equal(t, true, v.RoleAtCalled, "Expected RoleAt(%d) to be called", slot)
	assert.Equal(t, uint64(slot), v.RoleAtArg1, "RoleAt called with the wrong arg")
}

func TestAttests_NextSlot(t *testing.T) {
	attSubmitted := make(chan interface{})
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}, AttSubmitted: attSubmitted}
	ctx, cancel := context.WithCancel(context.Background())

	slot := primitives.Slot(55)
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	v.RolesAtRet = []iface.ValidatorRole{iface.RoleAttester}
	go func() {
		ticker <- slot

		cancel()
	}()
	run(ctx, v)
	<-attSubmitted
	require.Equal(t, true, v.AttestToBlockHeadCalled, "SubmitAttestation(%d) was not called", slot)
	assert.Equal(t, uint64(slot), v.AttestToBlockHeadArg1, "SubmitAttestation was called with wrong arg")
}

func TestProposes_NextSlot(t *testing.T) {
	blockProposed := make(chan interface{})
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}, BlockProposed: blockProposed}
	ctx, cancel := context.WithCancel(context.Background())

	slot := primitives.Slot(55)
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	v.RolesAtRet = []iface.ValidatorRole{iface.RoleProposer}
	go func() {
		ticker <- slot

		cancel()
	}()
	run(ctx, v)
	<-blockProposed
	require.Equal(t, true, v.ProposeBlockCalled, "ProposeBlock(%d) was not called", slot)
	assert.Equal(t, uint64(slot), v.ProposeBlockArg1, "ProposeBlock was called with wrong arg")
}

func TestBothProposesAndAttests_NextSlot(t *testing.T) {
	attSubmitted := make(chan interface{})
	blockProposed := make(chan interface{})
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}, AttSubmitted: attSubmitted, BlockProposed: blockProposed}
	ctx, cancel := context.WithCancel(context.Background())

	slot := primitives.Slot(55)
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	v.RolesAtRet = []iface.ValidatorRole{iface.RoleAttester, iface.RoleProposer}
	go func() {
		ticker <- slot

		cancel()
	}()
	run(ctx, v)
	<-attSubmitted
	<-blockProposed
	require.Equal(t, true, v.AttestToBlockHeadCalled, "SubmitAttestation(%d) was not called", slot)
	assert.Equal(t, uint64(slot), v.AttestToBlockHeadArg1, "SubmitAttestation was called with wrong arg")
	require.Equal(t, true, v.ProposeBlockCalled, "ProposeBlock(%d) was not called", slot)
	assert.Equal(t, uint64(slot), v.ProposeBlockArg1, "ProposeBlock was called with wrong arg")
}

func TestKeyReload_ActiveKey(t *testing.T) {
	ctx := context.Background()
	km := &mockKeymanager{}
	v := &testutil.FakeValidator{Km: km}
	ac := make(chan [][field_params.MLDSA87PubkeyLength]byte)
	current := [][field_params.MLDSA87PubkeyLength]byte{testutil.ActiveKey}
	onAccountsChanged(ctx, v, current, ac)
	assert.Equal(t, true, v.HandleKeyReloadCalled)
	// HandleKeyReloadCalled in the FakeValidator returns true if one of the keys is equal to the
	// ActiveKey. WaitForActivation is only called if none of the keys are active, so it shouldn't be called at all.
	assert.Equal(t, 0, v.WaitForActivationCalled)
}

func TestKeyReload_NoActiveKey(t *testing.T) {
	na := notActive(t)
	ctx := context.Background()
	km := &mockKeymanager{}
	v := &testutil.FakeValidator{Km: km}
	ac := make(chan [][field_params.MLDSA87PubkeyLength]byte)
	current := [][field_params.MLDSA87PubkeyLength]byte{na}
	onAccountsChanged(ctx, v, current, ac)
	assert.Equal(t, true, v.HandleKeyReloadCalled)
	// HandleKeyReloadCalled in the FakeValidator returns true if one of the keys is equal to the
	// ActiveKey. Since we are using a key we know is not active, it should return false, which
	// should cause the account change handler to call WaitForActivationCalled.
	assert.Equal(t, 1, v.WaitForActivationCalled)
}

func notActive(t *testing.T) [field_params.MLDSA87PubkeyLength]byte {
	var r [field_params.MLDSA87PubkeyLength]byte
	copy(r[:], testutil.ActiveKey[:])
	for i := range r {
		r[i] = bits.Reverse8(r[i])
	}
	require.DeepNotEqual(t, r, testutil.ActiveKey)
	return r
}

func TestUpdateProposerSettingsAt_EpochStart(t *testing.T) {
	feeRecipient, err := common.NewAddressFromString("Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046Fb65722E7b2455012BFEBf6177F1D2e9738D9")
	require.NoError(t, err)
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}}
	err = v.SetProposerSettings(context.Background(), &validatorserviceconfig.ProposerSettings{
		DefaultConfig: &validatorserviceconfig.ProposerOption{
			FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
				FeeRecipient: feeRecipient,
			},
		},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	hook := logTest.NewGlobal()
	slot := params.BeaconConfig().SlotsPerEpoch
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	go func() {
		ticker <- slot

		cancel()
	}()

	run(ctx, v)
	assert.LogsContain(t, hook, "updated proposer settings")
}

func TestUpdateProposerSettingsAt_EpochEndOk(t *testing.T) {
	feeRecipient, err := common.NewAddressFromString("Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046Fb65722E7b2455012BFEBf6177F1D2e9738D9")
	require.NoError(t, err)
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}, ProposerSettingWait: time.Duration(params.BeaconConfig().SecondsPerSlot-1) * time.Second}
	err = v.SetProposerSettings(context.Background(), &validatorserviceconfig.ProposerSettings{
		DefaultConfig: &validatorserviceconfig.ProposerOption{
			FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
				FeeRecipient: feeRecipient,
			},
		},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	hook := logTest.NewGlobal()
	slot := params.BeaconConfig().SlotsPerEpoch - 1 //have it set close to the end of epoch
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	go func() {
		ticker <- slot
		cancel()
	}()

	run(ctx, v)
	// can't test "Failed to update proposer settings" because of log.fatal
	assert.LogsContain(t, hook, "Mock updated proposer settings")
}

func TestUpdateProposerSettings_ContinuesAfterValidatorRegistrationFails(t *testing.T) {
	feeRecipient, err := common.NewAddressFromString("Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000046Fb65722E7b2455012BFEBf6177F1D2e9738D9")
	require.NoError(t, err)
	errSomeotherError := errors.New("some internal error")
	v := &testutil.FakeValidator{
		ProposerSettingsErr: errors.Wrap(ErrBuilderValidatorRegistration, errSomeotherError.Error()),
		Km:                  &mockKeymanager{accountsChangedFeed: &event.Feed{}},
	}
	err = v.SetProposerSettings(context.Background(), &validatorserviceconfig.ProposerSettings{
		DefaultConfig: &validatorserviceconfig.ProposerOption{
			FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
				FeeRecipient: feeRecipient,
			},
		},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	hook := logTest.NewGlobal()
	slot := params.BeaconConfig().SlotsPerEpoch
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	go func() {
		ticker <- slot

		cancel()
	}()
	run(ctx, v)
	assert.LogsContain(t, hook, ErrBuilderValidatorRegistration.Error())
}
