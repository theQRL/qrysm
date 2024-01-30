package testutil

import (
	"bytes"
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/theQRL/go-qrllib/dilithium"
	validatorserviceconfig "github.com/theQRL/qrysm/v4/config/validator/service"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	qrysmTime "github.com/theQRL/qrysm/v4/time"
	"github.com/theQRL/qrysm/v4/validator/client/iface"
	"github.com/theQRL/qrysm/v4/validator/keymanager"
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
	Balances                          map[[dilithium.CryptoPublicKeyBytes]byte]uint64
	IndexToPubkeyMap                  map[uint64][dilithium.CryptoPublicKeyBytes]byte
	PubkeyToIndexMap                  map[[dilithium.CryptoPublicKeyBytes]byte]uint64
	PubkeysToStatusesMap              map[[dilithium.CryptoPublicKeyBytes]byte]zondpb.ValidatorStatus
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
func (fv *FakeValidator) WaitForActivation(_ context.Context, accountChan chan [][dilithium.CryptoPublicKeyBytes]byte) error {
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
func (fv *FakeValidator) RolesAt(_ context.Context, slot primitives.Slot) (map[[dilithium.CryptoPublicKeyBytes]byte][]iface.ValidatorRole, error) {
	fv.RoleAtCalled = true
	fv.RoleAtArg1 = uint64(slot)
	vr := make(map[[dilithium.CryptoPublicKeyBytes]byte][]iface.ValidatorRole)
	vr[[dilithium.CryptoPublicKeyBytes]byte{1}] = fv.RolesAtRet
	return vr, nil
}

// SubmitAttestation for mocking.
func (fv *FakeValidator) SubmitAttestation(_ context.Context, slot primitives.Slot, _ [dilithium.CryptoPublicKeyBytes]byte) {
	fv.AttestToBlockHeadCalled = true
	fv.AttestToBlockHeadArg1 = uint64(slot)
}

// ProposeBlock for mocking.
func (fv *FakeValidator) ProposeBlock(_ context.Context, slot primitives.Slot, _ [dilithium.CryptoPublicKeyBytes]byte) {
	fv.ProposeBlockCalled = true
	fv.ProposeBlockArg1 = uint64(slot)
}

// SubmitAggregateAndProof for mocking.
func (_ *FakeValidator) SubmitAggregateAndProof(_ context.Context, _ primitives.Slot, _ [dilithium.CryptoPublicKeyBytes]byte) {
}

// SubmitSyncCommitteeMessage for mocking.
func (_ *FakeValidator) SubmitSyncCommitteeMessage(_ context.Context, _ primitives.Slot, _ [dilithium.CryptoPublicKeyBytes]byte) {
}

// LogAttestationsSubmitted for mocking.
func (_ *FakeValidator) LogAttestationsSubmitted() {}

// UpdateDomainDataCaches for mocking.
func (_ *FakeValidator) UpdateDomainDataCaches(context.Context, primitives.Slot) {}

// BalancesByPubkeys for mocking.
func (fv *FakeValidator) BalancesByPubkeys(_ context.Context) map[[dilithium.CryptoPublicKeyBytes]byte]uint64 {
	return fv.Balances
}

// IndicesToPubkeys for mocking.
func (fv *FakeValidator) IndicesToPubkeys(_ context.Context) map[uint64][dilithium.CryptoPublicKeyBytes]byte {
	return fv.IndexToPubkeyMap
}

// PubkeysToIndices for mocking.
func (fv *FakeValidator) PubkeysToIndices(_ context.Context) map[[dilithium.CryptoPublicKeyBytes]byte]uint64 {
	return fv.PubkeyToIndexMap
}

// PubkeysToStatuses for mocking.
func (fv *FakeValidator) PubkeysToStatuses(_ context.Context) map[[dilithium.CryptoPublicKeyBytes]byte]zondpb.ValidatorStatus {
	return fv.PubkeysToStatusesMap
}

// Keymanager for mocking
func (fv *FakeValidator) Keymanager() (keymanager.IKeymanager, error) {
	return fv.Km, nil
}

// CheckDoppelGanger for mocking
func (_ *FakeValidator) CheckDoppelGanger(_ context.Context) error {
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
func (fv *FakeValidator) HandleKeyReload(_ context.Context, newKeys [][dilithium.CryptoPublicKeyBytes]byte) (anyActive bool, err error) {
	fv.HandleKeyReloadCalled = true
	for _, key := range newKeys {
		if bytes.Equal(key[:], ActiveKey[:]) {
			return true, nil
		}
	}
	return false, nil
}

// SubmitSignedContributionAndProof for mocking
func (_ *FakeValidator) SubmitSignedContributionAndProof(_ context.Context, _ primitives.Slot, _ [dilithium.CryptoPublicKeyBytes]byte) {
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
func (_ *FakeValidator) SetPubKeyToValidatorIndexMap(_ context.Context, _ keymanager.IKeymanager) error {
	return nil
}

// SignValidatorRegistrationRequest for mocking
func (_ *FakeValidator) SignValidatorRegistrationRequest(_ context.Context, _ iface.SigningFunc, _ *zondpb.ValidatorRegistrationV1) (*zondpb.SignedValidatorRegistrationV1, error) {
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
