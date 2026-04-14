package collector

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/clear-route/vault-usage-exporter/pkg/vault"
	"github.com/prometheus/client_golang/prometheus"
)

const rootNamespace = "root"

type vaultClient interface {
	ListNamespaces(ctx context.Context) ([]string, error)
	ListAuthMethods(ctx context.Context, namespace string) ([]vault.Engine, error)
	ListSecretEngines(ctx context.Context, namespace string) ([]vault.Engine, error)
	GetMonthlyActivity(ctx context.Context) (*vault.MonthlyActivityData, error)
}

var (
	_ prometheus.Collector = (*Collector)(nil)
	_ vaultClient          = (*vault.Client)(nil)
)

type snapshot struct {
	namespaces      []string
	authMethods     []vault.Engine
	secretEngines   []vault.Engine
	monthlyActivity *vault.MonthlyActivityData
}

type refreshState struct {
	snapshot  *snapshot
	success   bool
	timestamp time.Time
	duration  time.Duration
}

type Option func(*Collector)

func WithTimeout(timeout time.Duration) Option {
	return func(c *Collector) {
		c.timeout = timeout
	}
}

func WithRefreshInterval(interval time.Duration) Option {
	return func(c *Collector) {
		c.refreshInterval = interval
	}
}

func WithVaultClient(client vaultClient) Option {
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
	vault   vaultClient
	rootCtx context.Context

	timeout         time.Duration
	refreshInterval time.Duration
	buildVersion    string

	buildInfo            *prometheus.Desc
	namespaceDesc        *prometheus.Desc
	secretEngineDesc     *prometheus.Desc
	authMethodDesc       *prometheus.Desc
	namespaceClientsDesc *prometheus.Desc
	mountClientsDesc     *prometheus.Desc
	refreshSuccessDesc   *prometheus.Desc
	refreshTimestampDesc *prometheus.Desc
	refreshDurationDesc  *prometheus.Desc

	mu    sync.RWMutex
	state refreshState
}

// New creates a new Collector with the provided options. It returns an error if required options are missing.
func New(opts ...Option) (*Collector, error) {
	c := &Collector{
		timeout:         5 * time.Second,
		refreshInterval: 5 * time.Minute,
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
		namespaceClientsDesc: prometheus.NewDesc(
			"vault_usage_namespace_clients",
			"Vault monthly client counts attributed to namespaces",
			[]string{"namespace", "namespace_id", "namespace_path", "client_type"},
			nil,
		),
		mountClientsDesc: prometheus.NewDesc(
			"vault_usage_mount_clients",
			"Vault monthly client counts attributed to mounts",
			[]string{"namespace", "namespace_id", "namespace_path", "mount_path", "mount_type", "client_type"},
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
			"Unix timestamp of last refresh attempt",
			nil,
			nil,
		),
		refreshDurationDesc: prometheus.NewDesc(
			"vault_usage_refresh_duration_seconds",
			"Duration of last refresh attempt in seconds",
			nil,
			nil,
		),
	}

	for _, opt := range opts {
		opt(c)
	}

	switch {
	case c.rootCtx == nil:
		return nil, fmt.Errorf("context is required")
	case c.vault == nil:
		return nil, fmt.Errorf("vault client is required")
	case c.timeout <= 0:
		return nil, fmt.Errorf("timeout must be greater than zero")
	case c.refreshInterval <= 0:
		return nil, fmt.Errorf("refresh interval must be greater than zero")
	}

	c.refresh(c.rootCtx)
	go c.run()

	return c, nil
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.namespaceDesc
	ch <- c.secretEngineDesc
	ch <- c.authMethodDesc
	ch <- c.namespaceClientsDesc
	ch <- c.mountClientsDesc
	ch <- c.refreshSuccessDesc
	ch <- c.refreshTimestampDesc
	ch <- c.refreshDurationDesc
	ch <- c.buildInfo
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(c.buildInfo, prometheus.GaugeValue, 1, c.buildVersion)

	state := c.getState()
	ch <- prometheus.MustNewConstMetric(c.refreshSuccessDesc, prometheus.GaugeValue, boolFloat(state.success))
	ch <- prometheus.MustNewConstMetric(c.refreshTimestampDesc, prometheus.GaugeValue, unixTimestamp(state.timestamp))
	ch <- prometheus.MustNewConstMetric(c.refreshDurationDesc, prometheus.GaugeValue, state.duration.Seconds())

	if state.snapshot == nil {
		return
	}

	for _, namespace := range state.snapshot.namespaces {
		ch <- prometheus.MustNewConstMetric(c.namespaceDesc, prometheus.GaugeValue, 1, namespace)
	}

	for _, authMethod := range state.snapshot.authMethods {
		ch <- prometheus.MustNewConstMetric(
			c.authMethodDesc,
			prometheus.GaugeValue,
			1,
			authMethod.Name,
			authMethod.Type,
			authMethod.Path,
			authMethod.Namespace,
		)
	}

	for _, secretEngine := range state.snapshot.secretEngines {
		ch <- prometheus.MustNewConstMetric(
			c.secretEngineDesc,
			prometheus.GaugeValue,
			1,
			secretEngine.Name,
			secretEngine.Type,
			secretEngine.Path,
			secretEngine.Namespace,
		)
	}

	for _, namespace := range state.snapshot.monthlyActivity.ByNamespace {
		emitClientCounts(
			ch,
			c.namespaceClientsDesc,
			namespace.Counts,
			namespaceLabel(namespace.NamespacePath),
			namespace.NamespaceID,
			namespace.NamespacePath,
		)

		for _, mount := range namespace.Mounts {
			emitClientCounts(
				ch,
				c.mountClientsDesc,
				mount.Counts,
				namespaceLabel(namespace.NamespacePath),
				namespace.NamespaceID,
				namespace.NamespacePath,
				mount.MountPath,
				strings.TrimSuffix(mount.MountType, "/"),
			)
		}
	}
}

func (c *Collector) run() {
	ticker := time.NewTicker(c.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.rootCtx.Done():
			return
		case <-ticker.C:
			c.refresh(c.rootCtx)
		}
	}
}

func (c *Collector) refresh(parent context.Context) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, c.timeout)
	defer cancel()

	nextState := refreshState{
		timestamp: start.UTC(),
	}

	snapshot, err := c.loadSnapshot(ctx)
	nextState.duration = time.Since(start)

	if err != nil {
		slog.Error("refresh failed", slog.String("error", err.Error()))

		c.mu.Lock()
		nextState.snapshot = c.state.snapshot
		c.state = nextState
		c.mu.Unlock()

		return
	}

	nextState.snapshot = snapshot
	nextState.success = true

	c.mu.Lock()
	c.state = nextState
	c.mu.Unlock()

	slog.Debug(
		"refresh completed",
		slog.Float64("duration_seconds", nextState.duration.Seconds()),
		slog.Int("namespaces", len(snapshot.namespaces)),
	)
}

func (c *Collector) loadSnapshot(ctx context.Context) (*snapshot, error) {
	namespaces, err := c.vault.ListNamespaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	namespaces = uniqueStrings(append([]string{rootNamespace}, namespaces...))

	authMethods := make([]vault.Engine, 0)
	secretEngines := make([]vault.Engine, 0)

	for _, namespace := range namespaces {
		auth, err := c.vault.ListAuthMethods(ctx, namespace)
		if err != nil {
			return nil, fmt.Errorf("list auth methods for namespace %q: %w", namespace, err)
		}

		authMethods = append(authMethods, auth...)

		engines, err := c.vault.ListSecretEngines(ctx, namespace)
		if err != nil {
			return nil, fmt.Errorf("list secret engines for namespace %q: %w", namespace, err)
		}

		secretEngines = append(secretEngines, engines...)
	}

	activity, err := c.vault.GetMonthlyActivity(ctx)
	if err != nil {
		return nil, fmt.Errorf("get monthly activity: %w", err)
	}

	if len(activity.ByNamespace) == 0 {
		slog.Info(
			"vault monthly activity is empty",
			slog.Int("clients", activity.Clients),
			slog.Int("entity_clients", activity.EntityClients),
			slog.Int("non_entity_clients", activity.NonEntityClients),
			slog.Int("secret_syncs", activity.SecretSyncs),
			slog.Int("acme_clients", activity.ACMEClients),
		)
	}

	return &snapshot{
		namespaces:      append([]string(nil), namespaces...),
		authMethods:     authMethods,
		secretEngines:   secretEngines,
		monthlyActivity: activity,
	}, nil
}

func (c *Collector) getState() refreshState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.state
}

func emitClientCounts(ch chan<- prometheus.Metric, desc *prometheus.Desc, counts vault.ClientCounts, labels ...string) {
	for _, metric := range []struct {
		name  string
		value int
	}{
		{name: "clients", value: counts.Clients},
		{name: "entity_clients", value: counts.EntityClients},
		{name: "non_entity_clients", value: counts.NonEntityClients},
		{name: "secret_syncs", value: counts.SecretSyncs},
		{name: "acme_clients", value: counts.ACMEClients},
	} {
		allLabels := append(append([]string(nil), labels...), metric.name)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(metric.value), allLabels...)
	}
}

func namespaceLabel(namespacePath string) string {
	namespace := strings.TrimSuffix(namespacePath, "/")
	if namespace == "" {
		return rootNamespace
	}

	return namespace
}

func boolFloat(v bool) float64 {
	if v {
		return 1
	}

	return 0
}

func unixTimestamp(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}

	return float64(t.Unix())
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))

	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}

		seen[value] = struct{}{}
		out = append(out, value)
	}

	return out
}
