package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a YAML-friendly time.Duration (e.g. "15s", "1m").
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration must be a scalar")
	}
	if value.Value == "" {
		d.Duration = 0
		return nil
	}

	dur, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	d.Duration = dur
	return nil
}
