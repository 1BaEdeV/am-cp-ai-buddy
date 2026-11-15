package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr             string
	DatabaseURL          string
	MLAgentURL           string
	IssuancePollInterval time.Duration
	DemoSeedOnStartup    bool
	MLToolsToken         string
}

// Load читает окружение процесса. Файл .env нужно предварительно загрузить в shell.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:             strings.TrimSpace(os.Getenv("HTTP_ADDR")),
		DatabaseURL:          strings.TrimSpace(os.Getenv("DATABASE_URL")),
		MLAgentURL:           strings.TrimSpace(os.Getenv("ML_AGENT_URL")),
		IssuancePollInterval: 500 * time.Millisecond,
		MLToolsToken:         strings.TrimSpace(os.Getenv("ML_TOOLS_TOKEN")),
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("задай DATABASE_URL для подключения к PostgreSQL")
	}
	if cfg.MLAgentURL == "" {
		cfg.MLAgentURL = "http://localhost:8090/chat"
	}
	if value := strings.TrimSpace(os.Getenv("ISSUANCE_POLL_INTERVAL")); value != "" {
		interval, err := time.ParseDuration(value)
		if err != nil || interval <= 0 {
			return Config{}, errors.New("ISSUANCE_POLL_INTERVAL должен быть положительной длительностью, например 500ms")
		}
		cfg.IssuancePollInterval = interval
	}
	if value := strings.TrimSpace(os.Getenv("DEMO_SEED_ON_STARTUP")); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, errors.New("DEMO_SEED_ON_STARTUP должен быть true или false")
		}
		cfg.DemoSeedOnStartup = enabled
	}
	return cfg, nil
}
