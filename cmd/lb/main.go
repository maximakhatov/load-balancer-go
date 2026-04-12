package main

import (
	"context"
	"flag"
	"load-balancer/internal/config"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

// TODO documentation
// TODO dry-run mode (just validate config and return error if invalid, so it can be used in CI/CD pipelines for linting)
func main() {
	configPath := flag.String("config", "configs/lb.example.yaml", "path to YAML config file")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	log.Printf("loaded config from %s (listen %s)", *configPath, cfg.Server.Listen)

	mux := http.NewServeMux()
	// TODO support of rewrite rules
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// TODO call lb logic instead
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("load balancer\n"))
	})

	srv := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("listening on %s", cfg.Server.Listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Print("shutting down...")

	// TODO ShutdownTimeout is a max timeout, but we can shutdown our LB right after all the in-flight requests are finished
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
