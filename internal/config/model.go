package config

type Config struct {
	Sources []SourceConfig `toml:"sources"`
	Outputs []OutputConfig `toml:"outputs"`
	State   StateConfig    `toml:"state"`
	Logging LoggingConfig  `toml:"logging"`
	Retry   RetryConfig    `toml:"retry"`
}

type SourceConfig struct {
	Type       string `toml:"type"`
	Name       string `toml:"name"`
	Endpoint   string `toml:"endpoint"`
	HostIP     string `toml:"host_ip"`
	BaseDomain string `toml:"base_domain"`
}

type OutputConfig struct {
	Type        string `toml:"type"`
	Name        string `toml:"name"`
	Enabled     *bool  `toml:"enabled"`
	URL         string `toml:"url"`
	Username    string `toml:"username"`
	Password    string `toml:"password"`
	PasswordRef string `toml:"password_ref"`
	ZoneID      string `toml:"zone_id"`
	APIKey      string `toml:"api_key"`
	APIKeyRef   string `toml:"api_key_ref"`
}

func (c OutputConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

type StateConfig struct {
	Path string `toml:"path"`
}

type LoggingConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

type RetryConfig struct {
	InitialInterval string `toml:"initial_interval"`
	MaxInterval     string `toml:"max_interval"`
	MaxElapsedTime  string `toml:"max_elapsed_time"`
}
