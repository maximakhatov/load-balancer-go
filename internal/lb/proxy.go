package lb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"load-balancer/internal/config"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"sync"
	"sync/atomic"
	"time"
)

type ProxyHandler struct {
	// Static backend params
	cfg      config.Config
	backends []*backend
	// Mutex for atomic updates
	mu sync.RWMutex
	// State for weighted RR
	weightSlots []int // map i -> backendId
	totalWeight int
	counter     uint64
}

type backend struct {
	// Static backend params
	url    *url.URL
	weight int
	proxy  *httputil.ReverseProxy
	// Mutex for atomic updates
	mu sync.RWMutex // TODO do we really need RW or simple Mutex is enough?
	// Current health state
	healthy           bool
	ConsecutiveOK     int
	ConsecutiveFailed int
}

func NewProxyHandler(cfg config.Config) (*ProxyHandler, error) {
	backends := make([]*backend, 0)
	weightSlots := make([]int, 0)
	totalWeight := 0

	for idx, upstream := range cfg.Upstreams {
		// Config loader ensures weight >= 1. Keep this check as a defensive guard
		if upstream.Weight < 1 {
			return nil, errors.New("invalid upstream weight: must be >= 1")
		}

		upstreamURL := upstream.URL
		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(upstreamURL)
				pr.SetXForwarded()
			},
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("proxy error (%s): %v", upstreamURL, err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}

		backends = append(backends, &backend{
			url:               upstreamURL,
			weight:            upstream.Weight,
			proxy:             proxy,
			healthy:           true,
			mu:                sync.RWMutex{},
			ConsecutiveOK:     0,
			ConsecutiveFailed: 0,
		})
		for i := 0; i < upstream.Weight; i++ {
			weightSlots = append(weightSlots, idx)
		}
		totalWeight += upstream.Weight
	}

	if len(backends) == 0 || totalWeight == 0 {
		return nil, errors.New("no active upstream targets")
	}

	return &ProxyHandler{
		cfg:         cfg,
		backends:    backends,
		mu:          sync.RWMutex{},
		weightSlots: weightSlots,
		totalWeight: totalWeight,
		counter:     0,
	}, nil
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := h.pickTarget()
	if target == nil {
		http.Error(w, "no healthy upstream", http.StatusServiceUnavailable)
		return
	}
	// TODO move header name and ability to disable it to config
	w.Header().Set("X-LB-Upstream", target.url.Host)
	target.proxy.ServeHTTP(w, r)
}

func (h *ProxyHandler) pickTarget() *backend {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.totalWeight == 0 {
		return nil
	}
	switch h.cfg.Balancer.Algorithm {
	case config.AlgorithmRandom:
		pick := rand.IntN(h.totalWeight)
		return h.pickByWeightIndex(pick)
	case config.AlgorithmRoundRobin:
		fallthrough
	default:
		next := atomic.AddUint64(&h.counter, 1) - 1
		pick := int(next % uint64(h.totalWeight))
		return h.pickByWeightIndex(pick)
	}
}

func (h *ProxyHandler) pickByWeightIndex(pick int) *backend {
	return h.backends[h.weightSlots[pick]]
}

func (h *ProxyHandler) Run(ctx context.Context) {
	if h.cfg.Health.Enabled {
		for _, backend := range h.backends {
			go func() {
				h.runHealthCheck(backend, ctx)
			}()
		}
	}
}

func (h *ProxyHandler) runHealthCheck(b *backend, ctx context.Context) {
	client := &http.Client{
		Timeout: h.cfg.Health.Timeout,
	}
	checkURL := *b.url
	checkURL.Path = path.Join(checkURL.Path, h.cfg.Health.Path)
	checkURLString := checkURL.String()

	ticker := time.NewTicker(h.cfg.Health.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() { // func() is used to ensure stateChanged is scoped to the single iteration of the loop
				stateChanged := false
				defer func() {
					if stateChanged {
						h.recalculateWeightSlots()
					}
				}()

				resp, err := client.Get(checkURLString)
				if err != nil {
					stateChanged = h.recordFailure(b, err.Error())
					return
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()

				if resp.StatusCode != h.cfg.Health.ExpectedStatus {
					stateChanged = h.recordFailure(b, fmt.Sprintf("expected status %d, got %d", h.cfg.Health.ExpectedStatus, resp.StatusCode))
					return
				}

				stateChanged = h.recordSuccess(b)
			}()
		}
	}
}

// Returns true if the backend is now healthy
func (h *ProxyHandler) recordSuccess(b *backend) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ConsecutiveOK++
	b.ConsecutiveFailed = 0
	if !b.healthy && b.ConsecutiveOK >= h.cfg.Health.HealthyThreshold {
		b.healthy = true
		return true
	}
	return false
}

// Returns true if the backend is now unhealthy
func (h *ProxyHandler) recordFailure(b *backend, lastError string) bool {
	fmt.Println("Health check failed for backend", b.url.Host, "with error", lastError)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ConsecutiveFailed++
	b.ConsecutiveOK = 0
	if b.healthy && b.ConsecutiveFailed >= h.cfg.Health.UnhealthyThreshold {
		b.healthy = false
		return true
	}
	return false
}

func (h *ProxyHandler) recalculateWeightSlots() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.weightSlots = make([]int, 0)
	h.totalWeight = 0
	h.counter = 0
	for idx, b := range h.backends {
		b.mu.RLock()
		if !b.healthy {
			b.mu.RUnlock()
			continue
		}
		for i := 0; i < b.weight; i++ {
			h.weightSlots = append(h.weightSlots, idx)
		}
		h.totalWeight += b.weight
		b.mu.RUnlock()
	}
}
