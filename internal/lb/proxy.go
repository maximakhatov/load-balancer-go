package lb

import (
	"errors"
	"load-balancer/internal/config"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
)

type ProxyHandler struct {
	algorithm config.BalancerAlgorithm
	targets   []weightedTarget
	counter   uint64
}

type weightedTarget struct {
	url   *url.URL
	proxy *httputil.ReverseProxy
}

// TODO rereadable config
func NewProxyHandler(cfg config.Config) (*ProxyHandler, error) {
	targets := make([]weightedTarget, 0)

	for _, upstream := range cfg.Upstreams {
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

		for i := 0; i < upstream.Weight; i++ {
			targets = append(targets, weightedTarget{
				url:   upstreamURL,
				proxy: proxy,
			})
		}
	}

	if len(targets) == 0 {
		return nil, errors.New("no active upstream targets after weight processing")
	}

	return &ProxyHandler{
		algorithm: cfg.Balancer.Algorithm,
		targets:   targets,
	}, nil
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := h.pickTarget()
	// TODO move header name and ability to disable it to config
	w.Header().Set("X-LB-Upstream", target.url.Host)
	target.proxy.ServeHTTP(w, r)
}

func (h *ProxyHandler) pickTarget() weightedTarget {
	switch h.algorithm {
	case config.AlgorithmRandom:
		return h.targets[rand.IntN(len(h.targets))]
	case config.AlgorithmRoundRobin:
		fallthrough
	default:
		next := atomic.AddUint64(&h.counter, 1) - 1
		return h.targets[next%uint64(len(h.targets))]
	}
}

func (h *ProxyHandler) TargetHosts() []string {
	hosts := make([]string, 0, len(h.targets))
	for _, target := range h.targets {
		hosts = append(hosts, target.url.Host)
	}
	return hosts
}
