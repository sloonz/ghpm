package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type NetworkConfig struct {
	TimeoutSeconds int `yaml:"timeoutSeconds"`
	Retries        int `yaml:"retries"`
}

type Config struct {
	PackagesDir string        `yaml:"packagesDir"`
	StateDir    string        `yaml:"stateDir"`
	CacheDir    string        `yaml:"cacheDir"`
	Network     NetworkConfig `yaml:"network"`
}

func DefaultConfig() Config {
	return Config{
		PackagesDir: "/var/lib/ghpm/packages",
		StateDir:    "/var/lib/ghpm/state",
		CacheDir:    "/var/cache/ghpm",
		Network: NetworkConfig{
			TimeoutSeconds: 30,
			Retries:        2,
		},
	}
}

func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) HTTPTimeout() time.Duration {
	if c.Network.TimeoutSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.Network.TimeoutSeconds) * time.Second
}

func (c Config) EnsureDirs(root string) error {
	dirs := []string{
		filepath.Join(root, c.PackagesDir),
		filepath.Join(root, c.StateDir),
		filepath.Join(root, c.StateDir, "work"),
		filepath.Join(root, c.CacheDir, "downloads"),
		filepath.Join(root, c.StateDir, "receipts"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}
