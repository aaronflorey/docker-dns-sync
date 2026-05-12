package config

type Config struct {
	Sources []SourceConfig `toml:"sources"`
	Outputs []OutputConfig `toml:"outputs"`
	State   StateConfig    `toml:"state"`
	Logging LoggingConfig  `toml:"logging"`
	Retry   RetryConfig    `toml:"retry"`
}

type SourceConfig struct {
	Type     string `toml:"type"`
	Name     string `toml:"name"`
	Endpoint string `toml:"endpoint"`
}

type OutputConfig struct {
	Type        string `toml:"type"`
	Name        string `toml:"name"`
	URL         string `toml:"url"`
	Username    string `toml:"username"`
	PasswordEnv string `toml:"password_env"`
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
