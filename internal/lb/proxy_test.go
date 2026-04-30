package lb

import (
	"load-balancer/internal/config"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func testHealthConfig() config.HealthConfig {
	return config.HealthConfig{
		Enabled:            true,
		Path:               "/health",
		Interval:           time.Second,
		Timeout:            time.Second,
		HealthyThreshold:   1,
		UnhealthyThreshold: 3,
		ExpectedStatus:     http.StatusOK,
	}
}

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

func TestProxyForwardsToUpstream(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream-ok"))
	}))
	t.Cleanup(up.Close)

	cfg := config.Config{
		Balancer: config.BalancerConfig{Algorithm: config.AlgorithmRoundRobin},
		Upstreams: []config.UpstreamConfig{
			{URL: mustParseURL(t, up.URL), Weight: 1},
		},
	}
	h, err := NewProxyHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://lb/test", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if got := rr.Body.String(); got != "upstream-ok" {
		t.Fatalf("body: %q", got)
	}
}

func TestRecordFailureUnhealthyThreshold(t *testing.T) {
	cfg := config.Config{
		Balancer: config.BalancerConfig{Algorithm: config.AlgorithmRoundRobin},
		Health: config.HealthConfig{
			HealthyThreshold:   1,
			UnhealthyThreshold: 3,
		},
		Upstreams: []config.UpstreamConfig{
			{URL: mustParseURL(t, "http://127.0.0.1:1"), Weight: 1},
		},
	}
	h, err := NewProxyHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b := h.backends[0]

	if transitioned := h.recordFailure(b, "e1"); transitioned {
		t.Fatal("first failure should not transition")
	}
	if transitioned := h.recordFailure(b, "e2"); transitioned {
		t.Fatal("second failure should not transition")
	}
	if !b.healthy {
		t.Fatal("backend should still be healthy")
	}
	if !h.recordFailure(b, "e3") {
		t.Fatal("third failure should mark unhealthy")
	}
	if b.healthy {
		t.Fatal("backend should be unhealthy")
	}
}

func TestRecordSuccessHealthyThresholdRecovery(t *testing.T) {
	cfg := config.Config{
		Balancer: config.BalancerConfig{Algorithm: config.AlgorithmRoundRobin},
		Health: config.HealthConfig{
			HealthyThreshold:   2,
			UnhealthyThreshold: 3,
		},
		Upstreams: []config.UpstreamConfig{
			{URL: mustParseURL(t, "http://127.0.0.1:1"), Weight: 1},
		},
	}
	h, err := NewProxyHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b := h.backends[0]
	b.mu.Lock()
	b.healthy = false
	b.ConsecutiveOK = 0
	b.ConsecutiveFailed = 0
	b.mu.Unlock()

	if h.recordSuccess(b) {
		t.Fatal("first success should not flip yet (threshold 2)")
	}
	if b.healthy {
		t.Fatal("should stay unhealthy after one success")
	}
	if !h.recordSuccess(b) {
		t.Fatal("second success should flip to healthy")
	}
	if !b.healthy {
		t.Fatal("should be healthy")
	}
}

func TestRecalculateWeightSlotsSkipsUnhealthy(t *testing.T) {
	cfg := config.Config{
		Balancer: config.BalancerConfig{Algorithm: config.AlgorithmRoundRobin},
		Health:   testHealthConfig(),
		Upstreams: []config.UpstreamConfig{
			{URL: mustParseURL(t, "http://127.0.0.1:9001"), Weight: 1},
			{URL: mustParseURL(t, "http://127.0.0.1:9002"), Weight: 1},
		},
	}
	h, err := NewProxyHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}

	h.backends[0].mu.Lock()
	h.backends[0].healthy = false
	h.backends[0].mu.Unlock()
	h.recalculateWeightSlots()

	if h.totalWeight != 1 {
		t.Fatalf("totalWeight: want 1, got %d", h.totalWeight)
	}
	if len(h.weightSlots) != 1 || h.weightSlots[0] != 1 {
		t.Fatalf("weightSlots: %+v", h.weightSlots)
	}
	for i := 0; i < 6; i++ {
		if h.pickTarget().url.Host != "127.0.0.1:9002" {
			t.Fatalf("pick %d: want only 9002", i)
		}
	}
}

func TestPickTargetAllUnhealthyReturnsNil(t *testing.T) {
	cfg := config.Config{
		Balancer: config.BalancerConfig{Algorithm: config.AlgorithmRoundRobin},
		Upstreams: []config.UpstreamConfig{
			{URL: mustParseURL(t, "http://127.0.0.1:9001"), Weight: 1},
		},
	}
	h, err := NewProxyHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.backends[0].mu.Lock()
	h.backends[0].healthy = false
	h.backends[0].mu.Unlock()
	h.recalculateWeightSlots()

	if h.pickTarget() != nil {
		t.Fatal("expected nil when no healthy backends")
	}
}

func TestServeHTTPNoHealthyUpstream503(t *testing.T) {
	cfg := config.Config{
		Balancer: config.BalancerConfig{Algorithm: config.AlgorithmRoundRobin},
		Upstreams: []config.UpstreamConfig{
			{URL: mustParseURL(t, "http://127.0.0.1:9001"), Weight: 1},
		},
	}
	h, err := NewProxyHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.backends[0].mu.Lock()
	h.backends[0].healthy = false
	h.backends[0].mu.Unlock()
	h.recalculateWeightSlots()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: %d", rr.Code)
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
