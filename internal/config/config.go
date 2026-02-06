package config

import (
	"fmt"
	"io"
	"time"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

const defaultCollectionInterval = 30 * time.Second

func Parse(r io.Reader) (*Config, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("cannot read file: %w", err)
	}

	var cfg Config

	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	if cfg.Exporter.CollectionInterval.Duration <= 0 {
		cfg.Exporter.CollectionInterval = Duration{Duration: defaultCollectionInterval}
	}

	v := validator.New()

	if err := v.Struct(cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	if cfg.Exporter.CollectionInterval.Duration <= 0 {
		return nil, fmt.Errorf("exporter.collection_interval must be > 0")
	}

	return &cfg, nil
}
