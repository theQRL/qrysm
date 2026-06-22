package gateway

import (
	"testing"

	"github.com/theQRL/qrysm/api/gateway"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestDefaultConfig(t *testing.T) {
	t.Run("Without debug endpoints", func(t *testing.T) {
		cfg := DefaultConfig(false, "qrl,qrysm")
		assert.NotNil(t, cfg.QRLPbMux.Mux)
		require.Equal(t, 1, len(cfg.QRLPbMux.Patterns))
		assert.Equal(t, "/internal/qrl/v1/", cfg.QRLPbMux.Patterns[0])
		assert.Equal(t, 4, len(cfg.QRLPbMux.Registrations))
		assert.NotNil(t, cfg.QrysmPbMux.Mux)
		require.Equal(t, 2, len(cfg.QrysmPbMux.Patterns))
		assert.Equal(t, "/qrl/v1alpha1/", cfg.QrysmPbMux.Patterns[0])
		assert.Equal(t, "/qrl/v1alpha2/", cfg.QrysmPbMux.Patterns[1])
		assert.Equal(t, 3, len(cfg.QrysmPbMux.Registrations))
	})

	t.Run("With debug endpoints", func(t *testing.T) {
		cfg := DefaultConfig(true, "qrl,qrysm")
		assert.NotNil(t, cfg.QRLPbMux.Mux)
		require.Equal(t, 1, len(cfg.QRLPbMux.Patterns))
		assert.Equal(t, "/internal/qrl/v1/", cfg.QRLPbMux.Patterns[0])
		assert.Equal(t, 5, len(cfg.QRLPbMux.Registrations))
		assert.NotNil(t, cfg.QrysmPbMux.Mux)
		require.Equal(t, 2, len(cfg.QrysmPbMux.Patterns))
		assert.Equal(t, "/qrl/v1alpha1/", cfg.QrysmPbMux.Patterns[0])
		assert.Equal(t, "/qrl/v1alpha2/", cfg.QrysmPbMux.Patterns[1])
		assert.Equal(t, 4, len(cfg.QrysmPbMux.Registrations))
	})
	t.Run("Without Qrysm API", func(t *testing.T) {
		cfg := DefaultConfig(true, "qrl")
		assert.NotNil(t, cfg.QRLPbMux.Mux)
		require.Equal(t, 1, len(cfg.QRLPbMux.Patterns))
		assert.Equal(t, "/internal/qrl/v1/", cfg.QRLPbMux.Patterns[0])
		assert.Equal(t, 5, len(cfg.QRLPbMux.Registrations))
		assert.Equal(t, (*gateway.PbMux)(nil), cfg.QrysmPbMux)
	})
	t.Run("Without QRL API", func(t *testing.T) {
		cfg := DefaultConfig(true, "qrysm")
		assert.Equal(t, (*gateway.PbMux)(nil), cfg.QRLPbMux)
		assert.NotNil(t, cfg.QrysmPbMux.Mux)
		require.Equal(t, 2, len(cfg.QrysmPbMux.Patterns))
		assert.Equal(t, "/qrl/v1alpha1/", cfg.QrysmPbMux.Patterns[0])
		assert.Equal(t, "/qrl/v1alpha2/", cfg.QrysmPbMux.Patterns[1])
		assert.Equal(t, 4, len(cfg.QrysmPbMux.Registrations))
	})
}
