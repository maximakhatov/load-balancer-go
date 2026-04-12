package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var f fileConfig
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return Config{}, fmt.Errorf("parse yaml: %w", err)
	}

	return parseFileConfig(f)
}
