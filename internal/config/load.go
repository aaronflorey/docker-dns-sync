package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

func Load(path string) (Config, error) {
	if path == "" {
		return Config{}, errors.New("config path is required")
	}

	if _, err := os.Stat(path); err != nil {
		return Config{}, fmt.Errorf("stat config file: %w", err)
	}

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config file: %w", err)
	}

	if err := Validate(cfg); err != nil {
		return Config{}, err
	}

	resolved, err := ResolveSecrets(cfg, os.LookupEnv)
	if err != nil {
		return Config{}, err
	}

	return resolved, nil
}
