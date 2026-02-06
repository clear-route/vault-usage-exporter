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

	"github.com/clear-route/vault-usage-exporter/internal/collector"
	customHTTP "github.com/clear-route/vault-usage-exporter/pkg/http"
	"github.com/clear-route/vault-usage-exporter/pkg/vault"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var version string

func main() {
	port := flag.String("port", "9090", "address for metrics HTTP server")
	address := flag.String("address", "0.0.0.0", "address for metrics HTTP server")

	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	slog.SetDefault(logger)

	vaultClient, err := vault.New()
	if err != nil {
		log.Fatalf("init vault client: %v", err)
	}

	slog.Info("authenticated successfully")

	c, err := collector.New(
		collector.WithContext(ctx),
		collector.WithTimeout(5*time.Second),
		collector.WithVaultClient(vaultClient),
		collector.WithBuildInfo(version),
	)
	if err != nil {
		log.Fatalf("error initializing collector: %v", err)
	}

	reg := prometheus.NewRegistry()

	reg.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{Namespace: "vault_usage"}),
		collectors.NewGoCollector(),
		c,
	)

	mux := &http.ServeMux{}

	mux.HandleFunc("/metrics", customHTTP.LoggingMiddleware(promhttp.HandlerFor(reg, promhttp.HandlerOpts{EnableOpenMetrics: false}).ServeHTTP))
	healthHandler := customHTTP.LoggingMiddleware(customHTTP.HealthHandler())
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/readyz", healthHandler)

	server := &http.Server{
		Addr:              *address + ":" + *port,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}

	go func() {
		slog.Info("start listening", slog.String("address", *address+":"+*port))

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
