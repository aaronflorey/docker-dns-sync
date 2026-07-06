package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

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
	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return Config{}, fmt.Errorf("decode config file: %w", err)
	}

	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		unknownKeys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			unknownKeys = append(unknownKeys, key.String())
		}

		return Config{}, fmt.Errorf("decode config file: unknown keys: %s", strings.Join(unknownKeys, ", "))
	}

	return cfg, nil
}
