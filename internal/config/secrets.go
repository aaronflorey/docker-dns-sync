package config

import (
	"errors"
	"fmt"
	"strings"
)

const envRefPrefix = "ENV:"

type LookupEnvFunc func(string) (string, bool)

func ResolveSecrets(cfg Config, lookup LookupEnvFunc) (Config, error) {
	if lookup == nil {
		return Config{}, errors.New("secret lookup is required")
	}

	resolved := cfg
	resolved.Outputs = append([]OutputConfig(nil), cfg.Outputs...)

	for i := range resolved.Outputs {
		if !resolved.Outputs[i].IsEnabled() {
			continue
		}

		ref := strings.TrimSpace(resolved.Outputs[i].PasswordRef)
		if ref != "" {
			name, err := parseEnvRef(ref)
			if err != nil {
				return Config{}, fmt.Errorf("outputs[%d].password_ref: %w", i, err)
			}

			value, ok := lookup(name)
			if !ok || strings.TrimSpace(value) == "" {
				return Config{}, fmt.Errorf("outputs[%d].password_ref references an unset environment variable", i)
			}

			resolved.Outputs[i].Password = value
			resolved.Outputs[i].PasswordRef = ""
		}

		ref = strings.TrimSpace(resolved.Outputs[i].APIKeyRef)
		if ref == "" {
			continue
		}

		name, err := parseEnvRef(ref)
		if err != nil {
			return Config{}, fmt.Errorf("outputs[%d].api_key_ref: %w", i, err)
		}

		value, ok := lookup(name)
		if !ok || strings.TrimSpace(value) == "" {
			return Config{}, fmt.Errorf("outputs[%d].api_key_ref references an unset environment variable", i)
		}

		resolved.Outputs[i].APIKey = value
		resolved.Outputs[i].APIKeyRef = ""
	}

	return resolved, nil
}

func parseEnvRef(ref string) (string, error) {
	if !strings.HasPrefix(ref, envRefPrefix) {
		return "", fmt.Errorf("must start with %q", envRefPrefix)
	}

	name := strings.TrimSpace(strings.TrimPrefix(ref, envRefPrefix))
	if name == "" {
		return "", errors.New("must include an environment variable name")
	}

	return name, nil
}
