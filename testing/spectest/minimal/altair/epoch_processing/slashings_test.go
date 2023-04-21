package epoch_processing

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/altair/epoch_processing"
)

func TestMinimal_Altair_EpochProcessing_Slashings(t *testing.T) {
	epoch_processing.RunSlashingsTests(t, "minimal")
}
