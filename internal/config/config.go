package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"time"
)

type BalancerAlgorithm string

const (
	AlgorithmRoundRobin BalancerAlgorithm = "round_robin"
	AlgorithmRandom     BalancerAlgorithm = "random"
)

type Config struct {
	Server    ServerConfig
	Balancer  BalancerConfig
	Upstreams []UpstreamConfig
	Health    HealthConfig
	Logging   LoggingConfig
}

type ServerConfig struct {
	Listen          string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

type BalancerConfig struct {
	Algorithm BalancerAlgorithm
}

type UpstreamConfig struct {
	URL    *url.URL
	Weight int
}

type HealthConfig struct {
	Enabled            bool
	Path               string
	Interval           time.Duration
	Timeout            time.Duration
	HealthyThreshold   int
	UnhealthyThreshold int
	ExpectedStatus     int
}

type LoggingConfig struct {
	Level slog.Level
}

type fileConfig struct {
	Server    fileServer     `yaml:"server"`
	Balancer  fileBalancer   `yaml:"balancer"`
	Upstreams []fileUpstream `yaml:"upstreams"`
	Health    fileHealth     `yaml:"health"`
	Logging   fileLogging    `yaml:"logging"`
}

type fileServer struct {
	Listen          string `yaml:"listen"`
	ReadTimeout     string `yaml:"read_timeout"`
	WriteTimeout    string `yaml:"write_timeout"`
	IdleTimeout     string `yaml:"idle_timeout"`
	ShutdownTimeout string `yaml:"shutdown_timeout"`
}

type fileBalancer struct {
	Algorithm string `yaml:"algorithm"`
}

type fileUpstream struct {
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
}

type fileHealth struct {
	Enabled            bool   `yaml:"enabled"`
	Path               string `yaml:"path"`
	Interval           string `yaml:"interval"`
	Timeout            string `yaml:"timeout"`
	HealthyThreshold   int    `yaml:"healthy_threshold"`
	UnhealthyThreshold int    `yaml:"unhealthy_threshold"`
	ExpectedStatus     int    `yaml:"expected_status"`
}

type fileLogging struct {
	Level string `yaml:"level"`
}

// Applies defaults, parses durations and URLs, and validates the config.
func parseFileConfig(f fileConfig) (Config, error) {
	applyDefaults(&f)

	readTO, err := time.ParseDuration(f.Server.ReadTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("server.read_timeout: %w", err)
	}
	writeTO, err := time.ParseDuration(f.Server.WriteTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("server.write_timeout: %w", err)
	}
	idleTO, err := time.ParseDuration(f.Server.IdleTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("server.idle_timeout: %w", err)
	}
	shutdownTO, err := time.ParseDuration(f.Server.ShutdownTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("server.shutdown_timeout: %w", err)
	}

	alg := BalancerAlgorithm(f.Balancer.Algorithm)
	switch alg {
	case AlgorithmRoundRobin, AlgorithmRandom:
	default:
		return Config{}, fmt.Errorf("balancer.algorithm: unknown %q (use round_robin or random)", f.Balancer.Algorithm)
	}

	if len(f.Upstreams) == 0 {
		return Config{}, fmt.Errorf("upstreams: at least one entry is required")
	}

	upstreams := make([]UpstreamConfig, 0, len(f.Upstreams))
	for i, u := range f.Upstreams {
		if u.URL == "" {
			return Config{}, fmt.Errorf("upstreams[%d].url: required", i)
		}
		parsed, err := url.Parse(u.URL)
		if err != nil {
			return Config{}, fmt.Errorf("upstreams[%d].url: %w", i, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return Config{}, fmt.Errorf("upstreams[%d].url: must include scheme and host (e.g. http://127.0.0.1:9001)", i)
		}
		w := u.Weight
		if w == 0 {
			w = 1
		}
		if w < 0 {
			return Config{}, fmt.Errorf("upstreams[%d].weight: must be >= 0", i)
		}
		upstreams = append(upstreams, UpstreamConfig{URL: parsed, Weight: w})
	}

	var (
		hInterval, hTimeout time.Duration
	)
	if f.Health.Enabled {
		hInterval, err = time.ParseDuration(f.Health.Interval)
		if err != nil {
			return Config{}, fmt.Errorf("health.interval: %w", err)
		}
		hTimeout, err = time.ParseDuration(f.Health.Timeout)
		if err != nil {
			return Config{}, fmt.Errorf("health.timeout: %w", err)
		}
	}

	level, err := parseLogLevel(f.Logging.Level)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Server: ServerConfig{
			Listen:          f.Server.Listen,
			ReadTimeout:     readTO,
			WriteTimeout:    writeTO,
			IdleTimeout:     idleTO,
			ShutdownTimeout: shutdownTO,
		},
		Balancer:  BalancerConfig{Algorithm: alg},
		Upstreams: upstreams,
		Health: HealthConfig{
			Enabled:            f.Health.Enabled,
			Path:               f.Health.Path,
			Interval:           hInterval,
			Timeout:            hTimeout,
			HealthyThreshold:   f.Health.HealthyThreshold,
			UnhealthyThreshold: f.Health.UnhealthyThreshold,
			ExpectedStatus:     f.Health.ExpectedStatus,
		},
		Logging: LoggingConfig{Level: level},
	}

	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyDefaults(f *fileConfig) {
	if f.Server.Listen == "" {
		f.Server.Listen = ":8080"
	}
	if f.Server.ReadTimeout == "" {
		f.Server.ReadTimeout = "30s"
	}
	if f.Server.WriteTimeout == "" {
		f.Server.WriteTimeout = "30s"
	}
	if f.Server.IdleTimeout == "" {
		f.Server.IdleTimeout = "120s"
	}
	if f.Server.ShutdownTimeout == "" {
		f.Server.ShutdownTimeout = "15s"
	}
	if f.Balancer.Algorithm == "" {
		f.Balancer.Algorithm = string(AlgorithmRoundRobin)
	}
	if f.Health.Path == "" {
		f.Health.Path = "/health"
	}
	if f.Health.Interval == "" {
		f.Health.Interval = "5s"
	}
	if f.Health.Timeout == "" {
		f.Health.Timeout = "2s"
	}
	if f.Health.HealthyThreshold == 0 {
		f.Health.HealthyThreshold = 1
	}
	if f.Health.UnhealthyThreshold == 0 {
		f.Health.UnhealthyThreshold = 3
	}
	if f.Health.ExpectedStatus == 0 {
		f.Health.ExpectedStatus = 200
	}
	if f.Logging.Level == "" {
		f.Logging.Level = "info"
	}
}

func validate(c Config) error {
	if c.Health.Enabled {
		if c.Health.Path == "" {
			return fmt.Errorf("health.path: required when health.enabled is true")
		}
		if c.Health.Interval <= 0 {
			return fmt.Errorf("health.interval: must be positive")
		}
		if c.Health.Timeout <= 0 {
			return fmt.Errorf("health.timeout: must be positive")
		}
		if c.Health.HealthyThreshold < 1 {
			return fmt.Errorf("health.healthy_threshold: must be >= 1")
		}
		if c.Health.UnhealthyThreshold < 1 {
			return fmt.Errorf("health.unhealthy_threshold: must be >= 1")
		}
		if c.Health.ExpectedStatus < 100 || c.Health.ExpectedStatus > 599 {
			return fmt.Errorf("health.expected_status: must be between 100 and 599")
		}
	}
	return nil
}

func parseLogLevel(s string) (slog.Level, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return l, fmt.Errorf("logging.level: %w", err)
	}
	return l, nil
}
