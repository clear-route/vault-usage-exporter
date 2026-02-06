package config

type Config struct {
	Server        ServerConfig   `yaml:"server" validate:"required"`
	AuthMethods   AuthMethods    `yaml:"auth_methods" validate:"required"`
	SecretEngines SecretEngines  `yaml:"secret_engines" validate:"required"`
	Exporter      ExporterConfig `yaml:"exporter"`
}

type ServerConfig struct {
	Address string `yaml:"address" validate:"required,url"`
}

type AuthMethods struct {
	Enabled bool `yaml:"enabled"`
}

type SecretEngines struct {
	Enabled bool `yaml:"enabled"`
}

type ExporterConfig struct {
	CollectionInterval Duration `yaml:"collection_interval"`
}
