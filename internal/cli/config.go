package cli

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the mycelium daemon configuration.
type Config struct {
	Listen       string `yaml:"listen"`
	NATSURL      string `yaml:"nats_url"`
	ForestServer string `yaml:"forest_server"`
	LandServer   string `yaml:"land_server"`
	BaseNATSPort int    `yaml:"base_nats_port"`
	BaseLandPort int    `yaml:"base_land_port"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if cfg.Listen == "" {
		cfg.Listen = ":8090"
	}
	if cfg.NATSURL == "" {
		cfg.NATSURL = "nats://127.0.0.1:4222"
	}
	if cfg.BaseNATSPort == 0 {
		cfg.BaseNATSPort = 4222
	}
	if cfg.BaseLandPort == 0 {
		cfg.BaseLandPort = 8080
	}

	return &cfg, nil
}
