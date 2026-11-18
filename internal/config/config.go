package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Database struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

type Config struct {
	Databases map[string]Database `yaml:"databases"`
}

func Load() (Config, error) {
	path := os.Getenv("TALKIN_MCP_CONFIG")
	if path == "" {
		path = filepath.Join("internal", "config", "config.yaml")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(contents, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	for _, name := range []string{"test", "prod_readonly"} {
		db, ok := cfg.Databases[name]
		if !ok || db.Host == "" || db.Port <= 0 || db.User == "" || db.Password == "" {
			return Config{}, fmt.Errorf("database configuration %q is incomplete", name)
		}
	}
	return cfg, nil
}
