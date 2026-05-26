package config

import (
	"errors"
	"fmt"
	"net/netip"
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

		hostIP := strings.TrimSpace(source.HostIP)
		if hostIP != "" {
			if _, err := netip.ParseAddr(hostIP); err != nil {
				return fmt.Errorf("sources[%d].host_ip must be a valid IP address", i)
			}
		}

		if baseDomain := normalizeDomainName(source.BaseDomain); strings.TrimSpace(source.BaseDomain) != "" && !isValidDomainName(baseDomain) {
			return fmt.Errorf("sources[%d].base_domain must be a valid domain name", i)
		}
	}

	for i, output := range cfg.Outputs {
		if strings.TrimSpace(output.Type) == "" {
			return fmt.Errorf("outputs[%d].type is required", i)
		}

		if strings.TrimSpace(output.Name) == "" {
			return fmt.Errorf("outputs[%d].name is required", i)
		}

		if !output.IsEnabled() {
			continue
		}

		switch strings.ToLower(strings.TrimSpace(output.Type)) {
		case "adguard":
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
		case "cloudflare":
			if strings.TrimSpace(output.ZoneID) == "" {
				return fmt.Errorf("outputs[%d].zone_id is required", i)
			}

			hasAPIKey := strings.TrimSpace(output.APIKey) != ""
			hasAPIKeyRef := strings.TrimSpace(output.APIKeyRef) != ""
			if hasAPIKey == hasAPIKeyRef {
				return fmt.Errorf("outputs[%d] must set exactly one of api_key or api_key_ref", i)
			}
		default:
			return fmt.Errorf("outputs[%d].type %q is not supported", i, output.Type)
		}
	}

	if strings.TrimSpace(cfg.State.Path) == "" {
		return errors.New("state.path is required")
	}

	level := strings.ToLower(strings.TrimSpace(cfg.Logging.Level))
	switch level {
	case "trace", "debug", "info", "warn", "error":
	default:
		return errors.New("logging.level must be one of trace, debug, info, warn, error")
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

func normalizeDomainName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	return strings.TrimSuffix(value, ".")
}

func isValidDomainName(value string) bool {
	if value == "" || strings.HasPrefix(value, ".") || strings.Contains(value, "..") {
		return false
	}

	for _, label := range strings.Split(value, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}

		for _, ch := range label {
			if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
				return false
			}
		}
	}

	return true
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
