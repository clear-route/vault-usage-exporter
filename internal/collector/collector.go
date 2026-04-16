package collector

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/clear-route/vault-client-count-exporter/pkg/vault"
	"github.com/prometheus/client_golang/prometheus"
)

const rootNamespace = "root"

type vaultClient interface {
	GetActivity(ctx context.Context, query vault.ActivityQuery) (*vault.MonthlyActivityData, error)
}

var (
	_ prometheus.Collector = (*Collector)(nil)
	_ vaultClient          = (*vault.Client)(nil)
)

type snapshot struct {
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

func WithActivityQuery(query vault.ActivityQuery) Option {
	return func(c *Collector) {
		c.activityQuery = query
	}
}

type Collector struct {
	vault   vaultClient
	rootCtx context.Context

	timeout         time.Duration
	refreshInterval time.Duration
	buildVersion    string
	activityQuery   vault.ActivityQuery

	buildInfo            *prometheus.Desc
	totalClientsDesc     *prometheus.Desc
	namespaceClientsDesc *prometheus.Desc
	mountClientsDesc     *prometheus.Desc
	currentNamespaceDesc *prometheus.Desc
	currentMountDesc     *prometheus.Desc
	activityPeriodDesc   *prometheus.Desc
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
			"vault_client_count_exporter_version",
			"Exporter Version",
			[]string{"version"},
			nil,
		),
		totalClientsDesc: prometheus.NewDesc(
			"vault_client_count_monthly_clients",
			"Vault monthly client counts by month",
			[]string{"start_time", "end_time", "month", "client_type"},
			nil,
		),
		namespaceClientsDesc: prometheus.NewDesc(
			"vault_client_count_monthly_namespace_clients",
			"Vault monthly client counts attributed to namespaces",
			[]string{"start_time", "end_time", "month", "namespace", "namespace_id", "namespace_path", "client_type"},
			nil,
		),
		mountClientsDesc: prometheus.NewDesc(
			"vault_client_count_monthly_mount_clients",
			"Vault monthly client counts attributed to mounts",
			[]string{"start_time", "end_time", "month", "namespace", "namespace_id", "namespace_path", "mount_path", "mount_type", "client_type"},
			nil,
		),
		currentNamespaceDesc: prometheus.NewDesc(
			"vault_client_count_current_namespace_clients",
			"Vault current snapshot client counts attributed to namespaces",
			[]string{"start_time", "end_time", "namespace", "namespace_id", "namespace_path", "client_type"},
			nil,
		),
		currentMountDesc: prometheus.NewDesc(
			"vault_client_count_current_mount_clients",
			"Vault current snapshot client counts attributed to mounts",
			[]string{"start_time", "end_time", "namespace", "namespace_id", "namespace_path", "mount_path", "mount_type", "client_type"},
			nil,
		),
		activityPeriodDesc: prometheus.NewDesc(
			"vault_client_count_activity_period_info",
			"Vault activity period metadata from the activity response",
			[]string{"start_time", "end_time"},
			nil,
		),
		refreshSuccessDesc: prometheus.NewDesc(
			"vault_client_count_refresh_success",
			"Whether the last refresh succeeded (1) or not (0)",
			nil,
			nil,
		),
		refreshTimestampDesc: prometheus.NewDesc(
			"vault_client_count_refresh_timestamp_seconds",
			"Unix timestamp of last refresh attempt",
			nil,
			nil,
		),
		refreshDurationDesc: prometheus.NewDesc(
			"vault_client_count_refresh_duration_seconds",
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
	ch <- c.totalClientsDesc
	ch <- c.namespaceClientsDesc
	ch <- c.mountClientsDesc
	ch <- c.currentNamespaceDesc
	ch <- c.currentMountDesc
	ch <- c.activityPeriodDesc
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

	ch <- prometheus.MustNewConstMetric(
		c.activityPeriodDesc,
		prometheus.GaugeValue,
		1,
		formatInfoTime(state.snapshot.monthlyActivity.StartTime),
		formatInfoTime(state.snapshot.monthlyActivity.EndTime),
	)

	startTimeLabel := formatInfoTime(state.snapshot.monthlyActivity.StartTime)
	endTimeLabel := formatInfoTime(state.snapshot.monthlyActivity.EndTime)

	for _, namespace := range state.snapshot.monthlyActivity.ByNamespace {
		emitClientCounts(
			ch,
			c.currentNamespaceDesc,
			namespace.Counts,
			startTimeLabel,
			endTimeLabel,
			namespaceLabel(namespace.NamespacePath),
			namespace.NamespaceID,
			namespace.NamespacePath,
		)

		for _, mount := range namespace.Mounts {
			emitClientCounts(
				ch,
				c.currentMountDesc,
				mount.Counts,
				startTimeLabel,
				endTimeLabel,
				namespaceLabel(namespace.NamespacePath),
				namespace.NamespaceID,
				namespace.NamespacePath,
				mount.MountPath,
				strings.TrimSuffix(mount.MountType, "/"),
			)
		}
	}

	for _, month := range monthlyBuckets(state.snapshot.monthlyActivity, state.timestamp) {
		monthLabel := formatMonthLabel(month.Timestamp)
		emitClientCounts(ch, c.totalClientsDesc, month.Counts, startTimeLabel, endTimeLabel, monthLabel)

		for _, namespace := range month.Namespaces {
			emitClientCounts(
				ch,
				c.namespaceClientsDesc,
				namespace.Counts,
				startTimeLabel,
				endTimeLabel,
				monthLabel,
				namespaceLabel(namespace.NamespacePath),
				namespace.NamespaceID,
				namespace.NamespacePath,
			)

			for _, mount := range namespace.Mounts {
				emitClientCounts(
					ch,
					c.mountClientsDesc,
					mount.Counts,
					startTimeLabel,
					endTimeLabel,
					monthLabel,
					namespaceLabel(namespace.NamespacePath),
					namespace.NamespaceID,
					namespace.NamespacePath,
					mount.MountPath,
					strings.TrimSuffix(mount.MountType, "/"),
				)
			}
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
		slog.Int("namespaces", len(snapshot.monthlyActivity.ByNamespace)),
		slog.Bool("monthly", c.activityQuery.Monthly),
		slog.String("start_time", c.activityQuery.StartTime),
		slog.String("end_time", c.activityQuery.EndTime),
	)
}

func (c *Collector) loadSnapshot(ctx context.Context) (*snapshot, error) {
	activity, err := c.vault.GetActivity(ctx, c.activityQuery)
	if err != nil {
		return nil, fmt.Errorf("get activity: %w", err)
	}

	if len(activity.ByNamespace) == 0 {
		slog.Info(
			"vault activity has no namespace attribution yet",
			slog.Int("clients", activity.Clients),
			slog.Int("entity_clients", activity.EntityClients),
			slog.Int("non_entity_clients", activity.NonEntityClients),
			slog.Int("secret_syncs", activity.SecretSyncs),
			slog.Int("acme_clients", activity.ACMEClients),
		)
	}

	return &snapshot{monthlyActivity: activity}, nil
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
		{name: "entity_clients", value: counts.EntityClients},
		{name: "non_entity_clients", value: counts.NonEntityClients},
		{name: "secret_syncs", value: counts.SecretSyncs},
		{name: "acme_clients", value: counts.ACMEClients},
	} {
		allLabels := append(append([]string(nil), labels...), metric.name)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(metric.value), allLabels...)
	}
}

func monthlyBuckets(activity *vault.MonthlyActivityData, fallbackTimestamp time.Time) []vault.MonthlyActivityMonth {
	if len(activity.Months) > 0 {
		return activity.Months
	}

	timestamp := fallbackTimestamp.UTC()
	if !activity.EndTime.IsZero() {
		timestamp = activity.EndTime.UTC()
	}

	return []vault.MonthlyActivityMonth{
		{
			Timestamp:  timestamp,
			Counts:     activity.ClientCounts,
			Namespaces: activity.ByNamespace,
		},
	}
}

func formatMonthLabel(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	return t.UTC().Format("2006-01")
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

func formatInfoTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	return t.UTC().Format(time.RFC3339)
}
