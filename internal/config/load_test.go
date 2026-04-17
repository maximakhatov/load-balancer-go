package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadExample(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "lb.example.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Listen != ":8080" {
		t.Fatalf("listen: got %q", cfg.Server.Listen)
	}
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Fatalf("read_timeout: got %q", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 30*time.Second {
		t.Fatalf("write_timeout: got %q", cfg.Server.WriteTimeout)
	}
	if cfg.Server.IdleTimeout != 120*time.Second {
		t.Fatalf("idle_timeout: got %q", cfg.Server.IdleTimeout)
	}
	if cfg.Server.ShutdownTimeout != 15*time.Second {
		t.Fatalf("shutdown_timeout: got %q", cfg.Server.ShutdownTimeout)
	}
	if cfg.Balancer.Algorithm != AlgorithmRoundRobin {
		t.Fatalf("algorithm: got %q", cfg.Balancer.Algorithm)
	}
	if len(cfg.Upstreams) != 3 {
		t.Fatalf("upstreams: want 3, got %d", len(cfg.Upstreams))
	}
	if cfg.Upstreams[0].URL.String() != "https://echo.free.beeceptor.com" {
		t.Fatalf("upstreams[0].url: got %q", cfg.Upstreams[0].URL.String())
	}
	if cfg.Upstreams[0].Weight != 1 {
		t.Fatalf("upstreams[0].weight: got %d", cfg.Upstreams[0].Weight)
	}
	if cfg.Upstreams[1].URL.String() != "http://127.0.0.1:9001" {
		t.Fatalf("upstreams[1].url: got %q", cfg.Upstreams[1].URL.String())
	}
	if cfg.Upstreams[1].Weight != 1 {
		t.Fatalf("upstreams[1].weight: got %d", cfg.Upstreams[1].Weight)
	}
	if cfg.Upstreams[2].URL.String() != "http://127.0.0.1:9002" {
		t.Fatalf("upstreams[2].url: got %q", cfg.Upstreams[2].URL.String())
	}
	if cfg.Upstreams[2].Weight != 2 {
		t.Fatalf("upstreams[2].weight: got %d", cfg.Upstreams[2].Weight)
	}
	if cfg.Upstreams[2].Weight != 2 {
		t.Fatalf("upstreams[2].weight: got %d", cfg.Upstreams[2].Weight)
	}
	if !cfg.Health.Enabled {
		t.Fatal("health should be enabled in example")
	}
	if cfg.Health.Path != "/health" {
		t.Fatalf("health.path: got %q", cfg.Health.Path)
	}
	if cfg.Health.Interval != 5*time.Second {
		t.Fatalf("health.interval: got %q", cfg.Health.Interval)
	}
	if cfg.Health.Timeout != 2*time.Second {
		t.Fatalf("health.timeout: got %q", cfg.Health.Timeout)
	}
	if cfg.Health.HealthyThreshold != 1 {
		t.Fatalf("health.healthy_threshold: got %d", cfg.Health.HealthyThreshold)
	}
	if cfg.Health.UnhealthyThreshold != 3 {
		t.Fatalf("health.unhealthy_threshold: got %d", cfg.Health.UnhealthyThreshold)
	}
	if cfg.Health.ExpectedStatus != 200 {
		t.Fatalf("health.expected_status: got %d", cfg.Health.ExpectedStatus)
	}
	if cfg.Logging.Level != slog.LevelInfo {
		t.Fatalf("logging.level: got %q", cfg.Logging.Level)
	}
}

func TestLoadMinimal(t *testing.T) {
	const yamlDoc = `
server:
  listen: ":9090"
balancer:
  algorithm: random
upstreams:
  - url: http://localhost:1
`
	dir := t.TempDir()
	p := filepath.Join(dir, "lb.yaml")
	if err := writeFile(p, yamlDoc); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Listen != ":9090" {
		t.Fatalf("listen: %q", cfg.Server.Listen)
	}
	if cfg.Balancer.Algorithm != AlgorithmRandom {
		t.Fatalf("algorithm: %q", cfg.Balancer.Algorithm)
	}
	if len(cfg.Upstreams) != 1 {
		t.Fatalf("upstreams: %d", len(cfg.Upstreams))
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
