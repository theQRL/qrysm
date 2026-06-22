package synccommittee

import (
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

var _ = Pool(&Store{})

// Pool defines the necessary methods for Qrysm sync pool to serve
// validators. In the current design, aggregated attestations
// are used by proposers and sync committee messages are used by
// sync aggregators.
type Pool interface {
	// Methods for Sync Contributions.
	SaveSyncCommitteeContribution(contr *qrysmpb.SyncCommitteeContribution) error
	SyncCommitteeContributions(slot primitives.Slot) ([]*qrysmpb.SyncCommitteeContribution, error)

	// Methods for Sync Committee Messages.
	SaveSyncCommitteeMessage(sig *qrysmpb.SyncCommitteeMessage) error
	SyncCommitteeMessages(slot primitives.Slot) ([]*qrysmpb.SyncCommitteeMessage, error)
}

// NewPool returns the sync committee store fulfilling the pool interface.
func NewPool() Pool {
	return NewStore()
}
