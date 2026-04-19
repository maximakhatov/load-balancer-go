package lb

import (
	"errors"
	"load-balancer/internal/config"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type ProxyHandler struct {
	algorithm config.BalancerAlgorithm
	backends  []*backend
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
	// Current state
	mu                sync.RWMutex
	healthy           bool
	ConsecutiveOK     int
	ConsecutiveFailed int
	LastCheckAt       time.Time
	LastError         string
}

// TODO rereadable config
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
			url:    upstreamURL,
			weight: upstream.Weight,
			proxy:  proxy,
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
		algorithm:   cfg.Balancer.Algorithm,
		backends:    backends,
		weightSlots: weightSlots,
		totalWeight: totalWeight,
		counter:     0,
	}, nil
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := h.pickTarget()
	// TODO move header name and ability to disable it to config
	w.Header().Set("X-LB-Upstream", target.url.Host)
	target.proxy.ServeHTTP(w, r)
}

func (h *ProxyHandler) pickTarget() *backend {
	switch h.algorithm {
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
