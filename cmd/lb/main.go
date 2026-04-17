package main

import (
	"context"
	"flag"
	"load-balancer/internal/config"
	"load-balancer/internal/lb"
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

	handler, err := lb.NewProxyHandler(cfg)
	if err != nil {
		log.Fatalf("failed to create balancer: %v", err)
	}

	srv := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      handler,
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
