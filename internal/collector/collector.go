package collector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/clear-route/vault-usage-exporter/pkg/vault"
	"github.com/prometheus/client_golang/prometheus"
)

type Option func(*Collector)

func WithTimeout(timeout time.Duration) Option {
	return func(c *Collector) {
		c.timeout = timeout
	}
}

func WithVaultClient(client *vault.Client) Option {
	return func(c *Collector) {
		c.vault = client
	}
}

func WithContext(ctx context.Context) Option {
	return func(c *Collector) {
		c.rootCtx = ctx
	}
}

func WithBuildInfo(version string) Option {
	return func(c *Collector) {
		c.buildVersion = version
	}
}

type Collector struct {
	vault   *vault.Client
	rootCtx context.Context

	timeout      time.Duration
	buildVersion string

	buildInfo            *prometheus.Desc
	namespaceDesc        *prometheus.Desc
	secretEngineDesc     *prometheus.Desc
	authMethodDesc       *prometheus.Desc
	leasesDesc           *prometheus.Desc
	tokensDesc           *prometheus.Desc
	refreshSuccessDesc   *prometheus.Desc
	refreshTimestampDesc *prometheus.Desc
	refreshDurationDesc  *prometheus.Desc
}

// Collector satisfies prometheus.Collector.
var _ prometheus.Collector = (*Collector)(nil)

// New creates a new Collector with the provided options. It returns an error if required options are missing.
func New(opts ...Option) (*Collector, error) {
	c := &Collector{
		buildInfo: prometheus.NewDesc(
			"vault_usage_exporter_version",
			"Exporter Version",
			[]string{"version"},
			nil,
		),
		namespaceDesc: prometheus.NewDesc(
			"vault_usage_namespaces",
			"Vault namespaces",
			[]string{"name"},
			nil,
		),
		secretEngineDesc: prometheus.NewDesc(
			"vault_usage_secret_engine",
			"Vault secret engines",
			[]string{"name", "type", "path", "namespace"},
			nil,
		),
		authMethodDesc: prometheus.NewDesc(
			"vault_usage_auth_method",
			"Vault auth methods",
			[]string{"name", "type", "path", "namespace"},
			nil,
		),
		leasesDesc: prometheus.NewDesc(
			"vault_usage_leases",
			"Vault leases count",
			[]string{"namespace"},
			nil,
		),
		tokensDesc: prometheus.NewDesc(
			"vault_usage_tokens",
			"Vault tokens count",
			[]string{"namespace"},
			nil,
		),
		refreshSuccessDesc: prometheus.NewDesc(
			"vault_usage_refresh_success",
			"Whether the last refresh succeeded (1) or not (0)",
			nil,
			nil,
		),
		refreshTimestampDesc: prometheus.NewDesc(
			"vault_usage_refresh_timestamp_seconds",
			"Unix timestamp of last refresh",
			nil,
			nil,
		),
		refreshDurationDesc: prometheus.NewDesc(
			"vault_usage_refresh_duration_seconds",
			"Duration of last refresh in seconds",
			nil,
			nil,
		),
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.rootCtx == nil {
		return nil, fmt.Errorf("context is required")
	}

	if c.vault == nil {
		return nil, fmt.Errorf("vault client is required")
	}

	return c, nil
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.namespaceDesc
	ch <- c.secretEngineDesc
	ch <- c.authMethodDesc
	ch <- c.leasesDesc
	ch <- c.tokensDesc
	ch <- c.refreshSuccessDesc
	ch <- c.refreshTimestampDesc
	ch <- c.refreshDurationDesc
	ch <- c.buildInfo
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	select {
	case <-c.rootCtx.Done():
		slog.Info("collector context done; skipping collection")

		return
	default:
	}

	slog.Debug("starting collection")
	start := time.Now()

	ctx, cancel := context.WithTimeout(c.rootCtx, c.timeout)
	defer cancel()

	ch <- prometheus.MustNewConstMetric(c.buildInfo, prometheus.GaugeValue, 1, c.buildVersion)

	// fetch all namespaces
	namespaces, err := c.vault.ListNamespaces(ctx)
	if err != nil {
		slog.Error("error listing namespaces", slog.String("error", err.Error()))
	}

	if len(namespaces) == 0 {
		namespaces = []string{"root"}
	}

	slog.Debug("finished fetching namespaces", slog.Int("count", len(namespaces)))

	// for each namespace, fetch auth methods, secret engines, leases and tokens count
	for _, ns := range namespaces {
		ch <- prometheus.MustNewConstMetric(c.namespaceDesc, prometheus.GaugeValue, 1, ns)

		// auth methods
		authMethods, err := c.vault.ListAuthMethods(ctx, ns)
		if err != nil {
			slog.Error("error listing auth methods", slog.String("namespace", ns), slog.String("error", err.Error()))
		}

		slog.Debug("finished fetching auth methods", slog.String("namespace", ns), slog.Int("count", len(authMethods)))

		for _, m := range authMethods {
			ch <- prometheus.MustNewConstMetric(
				c.authMethodDesc,
				prometheus.GaugeValue,
				1,
				m.Name,
				m.Type,
				m.Path,
				m.Namespace,
			)
		}

		// secret engines
		secretEngines, err := c.vault.ListSecretEngines(ctx, ns)
		if err != nil {
			slog.Error("error listing secret engines", slog.String("namespace", ns), slog.String("error", err.Error()))
		}

		slog.Debug("finished fetching secret engines", slog.String("namespace", ns), slog.Int("count", len(secretEngines)))

		for _, e := range secretEngines {
			ch <- prometheus.MustNewConstMetric(
				c.secretEngineDesc,
				prometheus.GaugeValue,
				1,
				e.Name,
				e.Type,
				e.Path,
				e.Namespace,
			)
		}

		// leases
		leases, err := c.vault.CountLeases(ctx, ns)
		if err != nil {
			slog.Error("error listing leases", slog.String("namespace", ns), slog.String("error", err.Error()))
		}

		slog.Debug("finished fetching leases", slog.String("namespace", ns), slog.Int("count", leases))

		ch <- prometheus.MustNewConstMetric(c.leasesDesc, prometheus.GaugeValue, float64(leases), ns)
	}

	duration := time.Since(start).Milliseconds()

	slog.Debug("finished collection", slog.Float64("duration_in_ms", float64(duration)))
}
