package operations

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/bellatrix/operations"
)

func TestMainnet_Bellatrix_Operations_SyncCommittee(t *testing.T) {
	operations.RunSyncCommitteeTest(t, "mainnet")
}
