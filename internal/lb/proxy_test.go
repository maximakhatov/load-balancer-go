package lb

import (
	"load-balancer/internal/config"
	"net/url"
	"testing"
)

func TestRoundRobinWeightedOrder(t *testing.T) {
	cfg := config.Config{
		Balancer: config.BalancerConfig{
			Algorithm: config.AlgorithmRoundRobin,
		},
		Upstreams: []config.UpstreamConfig{
			{URL: mustParseURL(t, "http://127.0.0.1:9001"), Weight: 1},
			{URL: mustParseURL(t, "http://127.0.0.1:9002"), Weight: 2},
		},
	}

	h, err := NewProxyHandler(cfg)
	if err != nil {
		t.Fatalf("NewProxyHandler: %v", err)
	}

	want := []string{
		"127.0.0.1:9001",
		"127.0.0.1:9002",
		"127.0.0.1:9002",
		"127.0.0.1:9001",
		"127.0.0.1:9002",
		"127.0.0.1:9002",
	}

	for i := range want {
		got := h.pickTarget().url.Host
		if got != want[i] {
			t.Fatalf("pick %d: got %q, want %q", i, got, want[i])
		}
	}
}

func TestNewProxyHandlerWithoutTargetsFails(t *testing.T) {
	cfg := config.Config{
		Balancer: config.BalancerConfig{Algorithm: config.AlgorithmRoundRobin},
		Upstreams: []config.UpstreamConfig{
			{URL: mustParseURL(t, "http://127.0.0.1:9001"), Weight: 0},
		},
	}

	_, err := NewProxyHandler(cfg)
	if err == nil {
		t.Fatal("expected error for no active targets")
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return parsed
}
