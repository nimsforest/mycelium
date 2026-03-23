package cli

import (
	"fmt"
	"os"

	"github.com/nimsforest/mycelium/internal/auth"
	"gopkg.in/yaml.v3"
)

// Config holds mycelium configuration.
type Config struct {
	Listen       string                             `yaml:"listen"`
	NATSURL      string                             `yaml:"nats_url"`
	DataDir      string                             `yaml:"data_dir"`
	OperatorName string                             `yaml:"operator_name"`
	Accounts     map[string]auth.AccountPermissions `yaml:"accounts"`
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
		cfg.Accounts = map[string]auth.AccountPermissions{
			"hub": {
				Publish:   []string{"*"},
				Subscribe: []string{"*"},
			},
		}
	}

	return cfg, nil
}
