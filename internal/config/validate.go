package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func Validate(cfg Config) error {
	if len(cfg.Sources) == 0 {
		return errors.New("at least one source must be configured")
	}

	if len(cfg.Outputs) == 0 {
		return errors.New("at least one output must be configured")
	}

	for i, source := range cfg.Sources {
		if strings.TrimSpace(source.Type) == "" {
			return fmt.Errorf("sources[%d].type is required", i)
		}

		if strings.TrimSpace(source.Name) == "" {
			return fmt.Errorf("sources[%d].name is required", i)
		}

		endpoint := strings.TrimSpace(source.Endpoint)
		if endpoint == "" {
			return fmt.Errorf("sources[%d].endpoint is required", i)
		}

		if !strings.HasPrefix(endpoint, "unix://") && !strings.HasPrefix(endpoint, "tcp://") {
			return fmt.Errorf("sources[%d].endpoint must use unix:// or tcp://", i)
		}
	}

	for i, output := range cfg.Outputs {
		if strings.TrimSpace(output.Type) == "" {
			return fmt.Errorf("outputs[%d].type is required", i)
		}

		if strings.TrimSpace(output.Name) == "" {
			return fmt.Errorf("outputs[%d].name is required", i)
		}

		if strings.TrimSpace(output.URL) == "" {
			return fmt.Errorf("outputs[%d].url is required", i)
		}

		if strings.TrimSpace(output.Username) == "" {
			return fmt.Errorf("outputs[%d].username is required", i)
		}

		hasPassword := strings.TrimSpace(output.Password) != ""
		hasPasswordRef := strings.TrimSpace(output.PasswordRef) != ""
		if hasPassword == hasPasswordRef {
			return fmt.Errorf("outputs[%d] must set exactly one of password or password_ref", i)
		}
	}

	if strings.TrimSpace(cfg.State.Path) == "" {
		return errors.New("state.path is required")
	}

	level := strings.ToLower(strings.TrimSpace(cfg.Logging.Level))
	switch level {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("logging.level must be one of debug, info, warn, error")
	}

	format := strings.ToLower(strings.TrimSpace(cfg.Logging.Format))
	switch format {
	case "json", "text":
	default:
		return errors.New("logging.format must be one of json or text")
	}

	if err := validateDuration("retry.initial_interval", cfg.Retry.InitialInterval); err != nil {
		return err
	}

	if err := validateDuration("retry.max_interval", cfg.Retry.MaxInterval); err != nil {
		return err
	}

	if err := validateDuration("retry.max_elapsed_time", cfg.Retry.MaxElapsedTime); err != nil {
		return err
	}

	return nil
}

func validateDuration(field string, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("%s must be a valid duration: %w", field, err)
	}

	if duration <= 0 {
		return fmt.Errorf("%s must be greater than zero", field)
	}

	return nil
}
