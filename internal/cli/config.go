package cli

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ExportConfig defines a NATS account export.
type ExportConfig struct {
	Name    string `yaml:"name"`
	Subject string `yaml:"subject"`
	Type    string `yaml:"type"` // "stream" or "service"
}

// ImportConfig defines a NATS account import from another account.
type ImportConfig struct {
	Name    string `yaml:"name"`
	Subject string `yaml:"subject"`
	Account string `yaml:"account"` // account name (resolved to public key at JWT build time)
	Type    string `yaml:"type"`    // "stream" or "service"
}

// AccountPermissions defines the NATS subject permissions for an account.
type AccountPermissions struct {
	Publish   []string       `yaml:"publish"`
	Subscribe []string       `yaml:"subscribe"`
	Exports   []ExportConfig `yaml:"exports"`
	Imports   []ImportConfig `yaml:"imports"`
}

// Config holds mycelium configuration.
type Config struct {
	Listen       string                        `yaml:"listen"`
	NATSURL      string                        `yaml:"nats_url"`
	DataDir      string                        `yaml:"data_dir"`
	OperatorName string                        `yaml:"operator_name"`
	Accounts     map[string]AccountPermissions `yaml:"accounts"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config %s: %w", path, err)
	}

	cfg := &Config{
		Listen:       ":8090",
		NATSURL:      "nats://127.0.0.1:4222",
		DataDir:      "/var/lib/mycelium",
		OperatorName: "nimsforest",
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if cfg.Accounts == nil {
		cfg.Accounts = map[string]AccountPermissions{
			"default": {
				Publish:   []string{"*"},
				Subscribe: []string{"*"},
			},
		}
	}

	return cfg, nil
}
