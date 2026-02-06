package exporter

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/clear-route/vault-usage-exporter/pkg/vault"
	"github.com/prometheus/client_golang/prometheus"
)

type Options struct {
	CollectAuthMethods   bool
	CollectSecretEngines bool
	Timeout              time.Duration
}

type Collector struct {
	vault   *vault.Client
	rootCtx context.Context
	opts    Options

	namespaceDesc        *prometheus.Desc
	secretEngineDesc     *prometheus.Desc
	authMethodDesc       *prometheus.Desc
	leasesDesc           *prometheus.Desc
	tokensDesc           *prometheus.Desc
	refreshSuccessDesc   *prometheus.Desc
	refreshTimestampDesc *prometheus.Desc
	refreshDurationDesc  *prometheus.Desc

	snap snapshot

	mu sync.Mutex
}

type snapshot struct {
	namespaces []string
	leases     map[string]int
	tokens     map[string]int

	secretEngines []vault.SecretEngine
	authMethods   []vault.AuthMethod

	lastRefresh         time.Time
	lastRefreshDuration time.Duration
	lastRefreshSuccess  bool
}

func NewCollector(rootCtx context.Context, vaultClient *vault.Client, opts Options) *Collector {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}

	return &Collector{
		vault:   vaultClient,
		rootCtx: rootCtx,
		opts:    opts,
		namespaceDesc: prometheus.NewDesc(
			"vault_usage_namespace",
			"Vault namespace present (1)",
			[]string{"name"},
			nil,
		),
		secretEngineDesc: prometheus.NewDesc(
			"vault_usage_secret_engine",
			"Vault secret engine present (1)",
			[]string{"name", "type", "path", "namespace"},
			nil,
		),
		authMethodDesc: prometheus.NewDesc(
			"vault_usage_auth_method",
			"Vault auth method present (1)",
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
		snap: snapshot{
			namespaces: []string{"root"},
			leases:     map[string]int{"root": 0},
			tokens:     map[string]int{"root": 0},
		},
	}
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
}

// Refresh collects data from Vault and updates the in-memory snapshot.
// Collect() uses this snapshot so scrapes do not hit Vault.
func (c *Collector) Refresh() error {
	if err := c.rootCtx.Err(); err != nil {
		return err
	}

	start := time.Now()

	ctx, cancel := context.WithTimeout(c.rootCtx, c.opts.Timeout)
	defer cancel()

	namespaces, err := c.vault.ListNamespaces(ctx)
	if err != nil {
		slog.Debug("list namespaces failed; falling back to root", slog.String("error", err.Error()))
		namespaces = []string{"root"}
	}

	leases := make(map[string]int, len(namespaces))
	tokens := make(map[string]int, len(namespaces))
	secretEngines := make([]vault.SecretEngine, 0)
	authMethods := make([]vault.AuthMethod, 0)

	var firstErr error

	for _, ns := range namespaces {
		leaseCount, lerr := c.vault.CountLeases(ctx, ns)
		if lerr != nil {
			if firstErr == nil {
				firstErr = lerr
			}
			leaseCount = 0
		}
		leases[ns] = leaseCount

		tokenCount, terr := c.vault.CountTokens(ctx, ns)
		if terr != nil {
			if firstErr == nil {
				firstErr = terr
			}
			tokenCount = 0
		}
		tokens[ns] = tokenCount

		if c.opts.CollectSecretEngines {
			engines, serr := c.vault.ListSecretEngines(ctx, ns)
			if serr != nil {
				if firstErr == nil {
					firstErr = serr
				}
			} else {
				secretEngines = append(secretEngines, engines...)
			}
		}

		if c.opts.CollectAuthMethods {
			methods, aerr := c.vault.ListAuthMethods(ctx, ns)
			if aerr != nil {
				if firstErr == nil {
					firstErr = aerr
				}
			} else {
				authMethods = append(authMethods, methods...)
			}
		}
	}

	end := time.Now()

	c.mu.Lock()
	c.snap = snapshot{
		namespaces:          namespaces,
		leases:              leases,
		tokens:              tokens,
		secretEngines:       secretEngines,
		authMethods:         authMethods,
		lastRefresh:         end,
		lastRefreshDuration: end.Sub(start),
		lastRefreshSuccess:  firstErr == nil,
	}
	c.mu.Unlock()

	if firstErr != nil {
		slog.Debug("refresh completed with errors", slog.String("error", firstErr.Error()))
		return firstErr
	}

	slog.Debug("metrics collection completed")

	return nil
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	snap := c.snap
	c.mu.Unlock()

	namespaces := append([]string(nil), snap.namespaces...)
	leases := make(map[string]int, len(snap.leases))
	for k, v := range snap.leases {
		leases[k] = v
	}
	tokens := make(map[string]int, len(snap.tokens))
	for k, v := range snap.tokens {
		tokens[k] = v
	}
	secretEngines := append([]vault.SecretEngine(nil), snap.secretEngines...)
	authMethods := append([]vault.AuthMethod(nil), snap.authMethods...)

	if snap.lastRefreshSuccess {
		ch <- prometheus.MustNewConstMetric(c.refreshSuccessDesc, prometheus.GaugeValue, 1)
	} else {
		ch <- prometheus.MustNewConstMetric(c.refreshSuccessDesc, prometheus.GaugeValue, 0)
	}
	if snap.lastRefresh.IsZero() {
		ch <- prometheus.MustNewConstMetric(c.refreshTimestampDesc, prometheus.GaugeValue, 0)
	} else {
		ch <- prometheus.MustNewConstMetric(c.refreshTimestampDesc, prometheus.GaugeValue, float64(snap.lastRefresh.Unix()))
	}
	ch <- prometheus.MustNewConstMetric(c.refreshDurationDesc, prometheus.GaugeValue, snap.lastRefreshDuration.Seconds())

	for _, ns := range namespaces {
		ch <- prometheus.MustNewConstMetric(c.namespaceDesc, prometheus.GaugeValue, 1, ns)
		ch <- prometheus.MustNewConstMetric(c.leasesDesc, prometheus.GaugeValue, float64(leases[ns]), ns)
		ch <- prometheus.MustNewConstMetric(c.tokensDesc, prometheus.GaugeValue, float64(tokens[ns]), ns)
	}

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
}

func (c *Collector) String() string {
	return fmt.Sprintf("vault usage collector (auth=%v, secretEngines=%v)", c.opts.CollectAuthMethods, c.opts.CollectSecretEngines)
}
