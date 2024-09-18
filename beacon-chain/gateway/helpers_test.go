package gateway

import (
	"testing"

	"github.com/theQRL/qrysm/api/gateway"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestDefaultConfig(t *testing.T) {
	t.Run("Without debug endpoints", func(t *testing.T) {
		cfg := DefaultConfig(false, "zond,qrysm")
		assert.NotNil(t, cfg.ZondPbMux.Mux)
		require.Equal(t, 1, len(cfg.ZondPbMux.Patterns))
		assert.Equal(t, "/internal/zond/v1/", cfg.ZondPbMux.Patterns[0])
		assert.Equal(t, 4, len(cfg.ZondPbMux.Registrations))
		assert.NotNil(t, cfg.V1AlphaPbMux.Mux)
		require.Equal(t, 2, len(cfg.V1AlphaPbMux.Patterns))
		assert.Equal(t, "/zond/v1alpha1/", cfg.V1AlphaPbMux.Patterns[0])
		assert.Equal(t, "/zond/v1alpha2/", cfg.V1AlphaPbMux.Patterns[1])
		assert.Equal(t, 3, len(cfg.V1AlphaPbMux.Registrations))
	})

	t.Run("With debug endpoints", func(t *testing.T) {
		cfg := DefaultConfig(true, "zond,qrysm")
		assert.NotNil(t, cfg.ZondPbMux.Mux)
		require.Equal(t, 1, len(cfg.ZondPbMux.Patterns))
		assert.Equal(t, "/internal/zond/v1/", cfg.ZondPbMux.Patterns[0])
		assert.Equal(t, 5, len(cfg.ZondPbMux.Registrations))
		assert.NotNil(t, cfg.V1AlphaPbMux.Mux)
		require.Equal(t, 2, len(cfg.V1AlphaPbMux.Patterns))
		assert.Equal(t, "/zond/v1alpha1/", cfg.V1AlphaPbMux.Patterns[0])
		assert.Equal(t, "/zond/v1alpha2/", cfg.V1AlphaPbMux.Patterns[1])
		assert.Equal(t, 4, len(cfg.V1AlphaPbMux.Registrations))
	})
	t.Run("Without Qrysm API", func(t *testing.T) {
		cfg := DefaultConfig(true, "zond")
		assert.NotNil(t, cfg.ZondPbMux.Mux)
		require.Equal(t, 1, len(cfg.ZondPbMux.Patterns))
		assert.Equal(t, "/internal/zond/v1/", cfg.ZondPbMux.Patterns[0])
		assert.Equal(t, 5, len(cfg.ZondPbMux.Registrations))
		assert.Equal(t, (*gateway.PbMux)(nil), cfg.V1AlphaPbMux)
	})
	t.Run("Without Eth API", func(t *testing.T) {
		cfg := DefaultConfig(true, "qrysm")
		assert.Equal(t, (*gateway.PbMux)(nil), cfg.ZondPbMux)
		assert.NotNil(t, cfg.V1AlphaPbMux.Mux)
		require.Equal(t, 2, len(cfg.V1AlphaPbMux.Patterns))
		assert.Equal(t, "/zond/v1alpha1/", cfg.V1AlphaPbMux.Patterns[0])
		assert.Equal(t, "/zond/v1alpha2/", cfg.V1AlphaPbMux.Patterns[1])
		assert.Equal(t, 4, len(cfg.V1AlphaPbMux.Registrations))
	})
}
