package testutil

import (
	"bytes"
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	validatorserviceconfig "github.com/theQRL/qrysm/config/validator/service"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	qrysmTime "github.com/theQRL/qrysm/time"
	"github.com/theQRL/qrysm/validator/client/iface"
	"github.com/theQRL/qrysm/validator/keymanager"
)

var _ iface.Validator = (*FakeValidator)(nil)

// FakeValidator for mocking.
type FakeValidator struct {
	DoneCalled                        bool
	WaitForWalletInitializationCalled bool
	SlasherReadyCalled                bool
	NextSlotCalled                    bool
	UpdateDutiesCalled                bool
	UpdateProtectionsCalled           bool
	RoleAtCalled                      bool
	AttestToBlockHeadCalled           bool
	ProposeBlockCalled                bool
	LogValidatorGainsAndLossesCalled  bool
	SaveProtectionsCalled             bool
	DeleteProtectionCalled            bool
	SlotDeadlineCalled                bool
	HandleKeyReloadCalled             bool
	WaitForChainStartCalled           int
	WaitForSyncCalled                 int
	WaitForActivationCalled           int
	CanonicalHeadSlotCalled           int
	ReceiveBlocksCalled               int
	RetryTillSuccess                  int
	ProposeBlockArg1                  uint64
	AttestToBlockHeadArg1             uint64
	RoleAtArg1                        uint64
	UpdateDutiesArg1                  uint64
	NextSlotRet                       <-chan primitives.Slot
	PublicKey                         string
	UpdateDutiesRet                   error
	ProposerSettingsErr               error
	RolesAtRet                        []iface.ValidatorRole
	Balances                          map[[field_params.DilithiumPubkeyLength]byte]uint64
	IndexToPubkeyMap                  map[uint64][field_params.DilithiumPubkeyLength]byte
	PubkeyToIndexMap                  map[[field_params.DilithiumPubkeyLength]byte]uint64
	PubkeysToStatusesMap              map[[field_params.DilithiumPubkeyLength]byte]zondpb.ValidatorStatus
	proposerSettings                  *validatorserviceconfig.ProposerSettings
	ProposerSettingWait               time.Duration
	Km                                keymanager.IKeymanager
}

// Done for mocking.
func (fv *FakeValidator) Done() {
	fv.DoneCalled = true
}

// WaitForKeymanagerInitialization for mocking.
func (fv *FakeValidator) WaitForKeymanagerInitialization(_ context.Context) error {
	fv.WaitForWalletInitializationCalled = true
	return nil
}

// LogSyncCommitteeMessagesSubmitted --
func (fv *FakeValidator) LogSyncCommitteeMessagesSubmitted() {}

// WaitForChainStart for mocking.
func (fv *FakeValidator) WaitForChainStart(_ context.Context) error {
	fv.WaitForChainStartCalled++
	if fv.RetryTillSuccess >= fv.WaitForChainStartCalled {
		return iface.ErrConnectionIssue
	}
	return nil
}

// WaitForActivation for mocking.
func (fv *FakeValidator) WaitForActivation(_ context.Context, accountChan chan [][field_params.DilithiumPubkeyLength]byte) error {
	fv.WaitForActivationCalled++
	if accountChan == nil {
		return nil
	}
	if fv.RetryTillSuccess >= fv.WaitForActivationCalled {
		return iface.ErrConnectionIssue
	}
	return nil
}

// WaitForSync for mocking.
func (fv *FakeValidator) WaitForSync(_ context.Context) error {
	fv.WaitForSyncCalled++
	if fv.RetryTillSuccess >= fv.WaitForSyncCalled {
		return iface.ErrConnectionIssue
	}
	return nil
}

// SlasherReady for mocking.
func (fv *FakeValidator) SlasherReady(_ context.Context) error {
	fv.SlasherReadyCalled = true
	return nil
}

// CanonicalHeadSlot for mocking.
func (fv *FakeValidator) CanonicalHeadSlot(_ context.Context) (primitives.Slot, error) {
	fv.CanonicalHeadSlotCalled++
	if fv.RetryTillSuccess > fv.CanonicalHeadSlotCalled {
		return 0, iface.ErrConnectionIssue
	}
	return 0, nil
}

// SlotDeadline for mocking.
func (fv *FakeValidator) SlotDeadline(_ primitives.Slot) time.Time {
	fv.SlotDeadlineCalled = true
	return qrysmTime.Now()
}

// NextSlot for mocking.
func (fv *FakeValidator) NextSlot() <-chan primitives.Slot {
	fv.NextSlotCalled = true
	return fv.NextSlotRet
}

// UpdateDuties for mocking.
func (fv *FakeValidator) UpdateDuties(_ context.Context, slot primitives.Slot) error {
	fv.UpdateDutiesCalled = true
	fv.UpdateDutiesArg1 = uint64(slot)
	return fv.UpdateDutiesRet
}

// UpdateProtections for mocking.
func (fv *FakeValidator) UpdateProtections(_ context.Context, _ uint64) error {
	fv.UpdateProtectionsCalled = true
	return nil
}

// LogValidatorGainsAndLosses for mocking.
func (fv *FakeValidator) LogValidatorGainsAndLosses(_ context.Context, _ primitives.Slot) error {
	fv.LogValidatorGainsAndLossesCalled = true
	return nil
}

// ResetAttesterProtectionData for mocking.
func (fv *FakeValidator) ResetAttesterProtectionData() {
	fv.DeleteProtectionCalled = true
}

// RolesAt for mocking.
func (fv *FakeValidator) RolesAt(_ context.Context, slot primitives.Slot) (map[[field_params.DilithiumPubkeyLength]byte][]iface.ValidatorRole, error) {
	fv.RoleAtCalled = true
	fv.RoleAtArg1 = uint64(slot)
	vr := make(map[[field_params.DilithiumPubkeyLength]byte][]iface.ValidatorRole)
	vr[[field_params.DilithiumPubkeyLength]byte{1}] = fv.RolesAtRet
	return vr, nil
}

// SubmitAttestation for mocking.
func (fv *FakeValidator) SubmitAttestation(_ context.Context, slot primitives.Slot, _ [field_params.DilithiumPubkeyLength]byte) {
	fv.AttestToBlockHeadCalled = true
	fv.AttestToBlockHeadArg1 = uint64(slot)
}

// ProposeBlock for mocking.
func (fv *FakeValidator) ProposeBlock(_ context.Context, slot primitives.Slot, _ [field_params.DilithiumPubkeyLength]byte) {
	fv.ProposeBlockCalled = true
	fv.ProposeBlockArg1 = uint64(slot)
}

// SubmitAggregateAndProof for mocking.
func (*FakeValidator) SubmitAggregateAndProof(_ context.Context, _ primitives.Slot, _ [field_params.DilithiumPubkeyLength]byte) {
}

// SubmitSyncCommitteeMessage for mocking.
func (*FakeValidator) SubmitSyncCommitteeMessage(_ context.Context, _ primitives.Slot, _ [field_params.DilithiumPubkeyLength]byte) {
}

// LogAttestationsSubmitted for mocking.
func (*FakeValidator) LogAttestationsSubmitted() {}

// UpdateDomainDataCaches for mocking.
func (*FakeValidator) UpdateDomainDataCaches(context.Context, primitives.Slot) {}

// BalancesByPubkeys for mocking.
func (fv *FakeValidator) BalancesByPubkeys(_ context.Context) map[[field_params.DilithiumPubkeyLength]byte]uint64 {
	return fv.Balances
}

// IndicesToPubkeys for mocking.
func (fv *FakeValidator) IndicesToPubkeys(_ context.Context) map[uint64][field_params.DilithiumPubkeyLength]byte {
	return fv.IndexToPubkeyMap
}

// PubkeysToIndices for mocking.
func (fv *FakeValidator) PubkeysToIndices(_ context.Context) map[[field_params.DilithiumPubkeyLength]byte]uint64 {
	return fv.PubkeyToIndexMap
}

// PubkeysToStatuses for mocking.
func (fv *FakeValidator) PubkeysToStatuses(_ context.Context) map[[field_params.DilithiumPubkeyLength]byte]zondpb.ValidatorStatus {
	return fv.PubkeysToStatusesMap
}

// Keymanager for mocking
func (fv *FakeValidator) Keymanager() (keymanager.IKeymanager, error) {
	return fv.Km, nil
}

// CheckDoppelGanger for mocking
func (*FakeValidator) CheckDoppelGanger(_ context.Context) error {
	return nil
}

// ReceiveBlocks for mocking
func (fv *FakeValidator) ReceiveBlocks(_ context.Context, connectionErrorChannel chan<- error) {
	fv.ReceiveBlocksCalled++
	if fv.RetryTillSuccess > fv.ReceiveBlocksCalled {
		connectionErrorChannel <- iface.ErrConnectionIssue
	}
}

// HandleKeyReload for mocking
func (fv *FakeValidator) HandleKeyReload(_ context.Context, newKeys [][field_params.DilithiumPubkeyLength]byte) (anyActive bool, err error) {
	fv.HandleKeyReloadCalled = true
	for _, key := range newKeys {
		if bytes.Equal(key[:], ActiveKey[:]) {
			return true, nil
		}
	}
	return false, nil
}

// SubmitSignedContributionAndProof for mocking
func (*FakeValidator) SubmitSignedContributionAndProof(_ context.Context, _ primitives.Slot, _ [field_params.DilithiumPubkeyLength]byte) {
}

// HasProposerSettings for mocking
func (*FakeValidator) HasProposerSettings() bool {
	return true
}

// PushProposerSettings for mocking
func (fv *FakeValidator) PushProposerSettings(ctx context.Context, km keymanager.IKeymanager, slot primitives.Slot, deadline time.Time) error {
	nctx, cancel := context.WithDeadline(ctx, deadline)
	ctx = nctx
	defer cancel()
	time.Sleep(fv.ProposerSettingWait)
	if ctx.Err() == context.DeadlineExceeded {
		log.Error("deadline exceeded")
		// can't return error or it will trigger a log.fatal
		return nil
	}

	if fv.ProposerSettingsErr != nil {
		return fv.ProposerSettingsErr
	}

	log.Infoln("Mock updated proposer settings")
	return nil
}

// SetPubKeyToValidatorIndexMap for mocking
func (*FakeValidator) SetPubKeyToValidatorIndexMap(_ context.Context, _ keymanager.IKeymanager) error {
	return nil
}

// SignValidatorRegistrationRequest for mocking
func (*FakeValidator) SignValidatorRegistrationRequest(_ context.Context, _ iface.SigningFunc, _ *zondpb.ValidatorRegistrationV1) (*zondpb.SignedValidatorRegistrationV1, error) {
	return nil, nil
}

// ProposerSettings for mocking
func (f *FakeValidator) ProposerSettings() *validatorserviceconfig.ProposerSettings {
	return f.proposerSettings
}

// SetProposerSettings for mocking
func (f *FakeValidator) SetProposerSettings(_ context.Context, settings *validatorserviceconfig.ProposerSettings) error {
	f.proposerSettings = settings
	return nil
}
