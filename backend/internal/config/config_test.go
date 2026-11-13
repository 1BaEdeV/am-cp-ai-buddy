package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", " postgres://demo:demo@localhost/demo ")
	t.Setenv("HTTP_ADDR", " ")
	t.Setenv("ISSUANCE_POLL_INTERVAL", "")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, ":8080", cfg.HTTPAddr)
	assert.Equal(t, "postgres://demo:demo@localhost/demo", cfg.DatabaseURL)
	assert.Equal(t, 500*time.Millisecond, cfg.IssuancePollInterval)
}

func TestLoadIssuancePollInterval(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  time.Duration
	}{
		{"250ms", 250 * time.Millisecond}, {" 2s ", 2 * time.Second}, {"0.5s", 500 * time.Millisecond}, {" \t", 500 * time.Millisecond},
	} {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://localhost/demo")
			t.Setenv("HTTP_ADDR", " :8090 ")
			t.Setenv("ISSUANCE_POLL_INTERVAL", tt.value)
			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, ":8090", cfg.HTTPAddr)
			assert.Equal(t, tt.want, cfg.IssuancePollInterval)
		})
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	for _, value := range []string{"0s", "-1s", "500", "invalid", "999999999999999999999h"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://localhost/demo")
			t.Setenv("ISSUANCE_POLL_INTERVAL", value)
			cfg, err := Load()
			require.ErrorContains(t, err, "ISSUANCE_POLL_INTERVAL")
			assert.Empty(t, cfg)
		})
	}
	t.Setenv("DATABASE_URL", " \t")
	cfg, err := Load()
	require.ErrorContains(t, err, "DATABASE_URL")
	assert.Empty(t, cfg)
}
