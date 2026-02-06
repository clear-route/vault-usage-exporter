package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/clear-route/vault-usage-exporter/internal/config"
	"github.com/clear-route/vault-usage-exporter/internal/exporter"
	customHTTP "github.com/clear-route/vault-usage-exporter/pkg/http"
	"github.com/clear-route/vault-usage-exporter/pkg/vault"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config yaml")
	listenAddress := flag.String("listen-address", ":9090", "address for metrics HTTP server")

	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	slog.SetDefault(logger)

	f, err := os.Open(*configPath)
	if err != nil {
		log.Fatalf("open config file: %v", err)
	}

	defer func() {
		_ = f.Close()
	}()

	cfg, err := config.Parse(f)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	slog.Info("parsed config successfully config file", slog.String("path", *configPath))

	vaultClient, err := vault.New(cfg.Server.Address)
	if err != nil {
		log.Fatalf("init vault client: %v", err)
	}

	slog.Info("authenticated successfully", slog.String("vault_address", cfg.Server.Address))

	collector := exporter.NewCollector(ctx, vaultClient, exporter.Options{
		CollectAuthMethods:   cfg.AuthMethods.Enabled,
		CollectSecretEngines: cfg.SecretEngines.Enabled,
	})

	reg := prometheus.NewRegistry()

	reg.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{Namespace: "vault_usage"}),
		collectors.NewGoCollector(),
		collector,
	)

	collectionInterval := cfg.Exporter.CollectionInterval.Duration
	slog.Info("starting periodic collection", slog.Duration("interval", collectionInterval))

	if err := collector.Refresh(); err != nil {
		slog.Warn("initial collection failed", slog.String("error", err.Error()))
	} else {
		slog.Info("initial collection succeeded")
	}

	go func() {
		ticker := time.NewTicker(collectionInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := collector.Refresh(); err != nil {
					slog.Warn("periodic collection failed", slog.String("error", err.Error()))
				}
			}
		}
	}()

	mux := &http.ServeMux{}

	mux.HandleFunc("/metrics", customHTTP.LoggingMiddleware(promhttp.HandlerFor(reg, promhttp.HandlerOpts{EnableOpenMetrics: false}).ServeHTTP))
	healthHandler := customHTTP.LoggingMiddleware(customHTTP.HealthHandler())
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/readyz", healthHandler)

	server := &http.Server{
		Addr:              "0.0.0.0:9090",
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}

	go func() {
		slog.Info("start listening", slog.String("address", *listenAddress))

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("error while listening", slog.String("error", err.Error()))
		}
	}()

	<-ctx.Done()
	slog.Info("received shutdown signal")

	//nolint: mnd
	shutdownCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("error while shutting down server: %v", err)
	}

	slog.Info("Exiting")
}
