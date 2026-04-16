package collector

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/clear-route/vault-client-count-exporter/pkg/vault"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

type fakeVaultClient struct {
	activity *vault.MonthlyActivityData
	err      error

	getActivityCalls int
	lastQuery        vault.ActivityQuery
}

func (f *fakeVaultClient) GetActivity(_ context.Context, query vault.ActivityQuery) (*vault.MonthlyActivityData, error) {
	f.getActivityCalls++
	f.lastQuery = query
	if f.err != nil {
		return nil, f.err
	}

	if f.activity == nil {
		return &vault.MonthlyActivityData{}, nil
	}

	copyValue := *f.activity
	copyValue.ByNamespace = append([]vault.MonthlyActivityNamespace(nil), f.activity.ByNamespace...)
	copyValue.Months = append([]vault.MonthlyActivityMonth(nil), f.activity.Months...)

	return &copyValue, nil
}

func TestCollectUsesCachedSnapshotAndEmitsMonthlyMetrics(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &fakeVaultClient{
		activity: &vault.MonthlyActivityData{
			StartTime: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, time.March, 31, 23, 59, 59, 0, time.UTC),
			ByNamespace: []vault.MonthlyActivityNamespace{
				{
					NamespaceID:   "root",
					NamespacePath: "",
					Counts: vault.ClientCounts{
						Clients:          13,
						EntityClients:    9,
						NonEntityClients: 2,
						SecretSyncs:      1,
						ACMEClients:      1,
					},
					Mounts: []vault.MonthlyActivityMount{
						{
							MountPath: "auth/token/",
							MountType: "token/",
							Counts: vault.ClientCounts{
								Clients:          13,
								EntityClients:    9,
								NonEntityClients: 2,
								SecretSyncs:      1,
								ACMEClients:      1,
							},
						},
					},
				},
				{
					NamespaceID:   "ns-1",
					NamespacePath: "team-a/",
					Counts: vault.ClientCounts{
						Clients:          8,
						EntityClients:    5,
						NonEntityClients: 1,
						SecretSyncs:      1,
						ACMEClients:      1,
					},
					Mounts: []vault.MonthlyActivityMount{
						{
							MountPath: "auth/approle/",
							MountType: "approle/",
							Counts: vault.ClientCounts{
								Clients:          8,
								EntityClients:    5,
								NonEntityClients: 1,
								SecretSyncs:      1,
								ACMEClients:      1,
							},
						},
					},
				},
			},
			Months: []vault.MonthlyActivityMonth{
				{
					Timestamp: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
					Counts: vault.ClientCounts{
						Clients:          11,
						EntityClients:    7,
						NonEntityClients: 2,
						SecretSyncs:      1,
						ACMEClients:      1,
					},
					Namespaces: []vault.MonthlyActivityNamespace{
						{
							NamespaceID:   "root",
							NamespacePath: "",
							Counts: vault.ClientCounts{
								Clients:          5,
								EntityClients:    3,
								NonEntityClients: 1,
								SecretSyncs:      1,
							},
							Mounts: []vault.MonthlyActivityMount{
								{
									MountPath: "auth/token/",
									MountType: "token/",
									Counts: vault.ClientCounts{
										Clients:          5,
										EntityClients:    3,
										NonEntityClients: 1,
										SecretSyncs:      1,
									},
								},
							},
						},
						{
							NamespaceID:   "ns-1",
							NamespacePath: "team-a/",
							Counts: vault.ClientCounts{
								Clients:          6,
								EntityClients:    4,
								NonEntityClients: 1,
								ACMEClients:      1,
							},
							Mounts: []vault.MonthlyActivityMount{
								{
									MountPath: "deleted mount; accessor \"auth_approle_deadbeef\"",
									MountType: "deleted mount",
									Counts: vault.ClientCounts{
										Clients:          6,
										EntityClients:    4,
										NonEntityClients: 1,
										ACMEClients:      1,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	c, err := New(
		WithContext(ctx),
		WithTimeout(250*time.Millisecond),
		WithRefreshInterval(time.Hour),
		WithBuildInfo("test-version"),
		WithVaultClient(client),
		WithActivityQuery(vault.ActivityQuery{
			StartTime: "2026-01-01T00:00:00Z",
			EndTime:   "2026-03-31T23:59:59Z",
			Monthly:   true,
		}),
	)
	require.NoError(t, err)

	initialCalls := client.getActivityCalls
	families := gatherMetricFamilies(t, c)
	require.Equal(t, initialCalls, client.getActivityCalls, "Collect should only read cached state")
	require.Equal(t, vault.ActivityQuery{
		StartTime: "2026-01-01T00:00:00Z",
		EndTime:   "2026-03-31T23:59:59Z",
		Monthly:   true,
	}, client.lastQuery)

	requireMetricValue(t, families, "vault_client_count_refresh_success", nil, 1)
	requireMetricValue(t, families, "vault_client_count_activity_period_info", map[string]string{
		"start_time": "2026-01-01T00:00:00Z",
		"end_time":   "2026-03-31T23:59:59Z",
	}, 1)
	requireMetricAbsent(t, families, "vault_client_count_monthly_clients", map[string]string{
		"start_time":  "2026-01-01T00:00:00Z",
		"end_time":    "2026-03-31T23:59:59Z",
		"month":       "2026-03",
		"client_type": "clients",
	})
	requireMetricValue(t, families, "vault_client_count_monthly_clients", map[string]string{
		"start_time":  "2026-01-01T00:00:00Z",
		"end_time":    "2026-03-31T23:59:59Z",
		"month":       "2026-03",
		"client_type": "acme_clients",
	}, 1)
	requireMetricValue(t, families, "vault_client_count_monthly_namespace_clients", map[string]string{
		"start_time":     "2026-01-01T00:00:00Z",
		"end_time":       "2026-03-31T23:59:59Z",
		"month":          "2026-03",
		"namespace":      "root",
		"namespace_id":   "root",
		"namespace_path": "",
		"client_type":    "entity_clients",
	}, 3)
	requireMetricValue(t, families, "vault_client_count_monthly_namespace_clients", map[string]string{
		"start_time":     "2026-01-01T00:00:00Z",
		"end_time":       "2026-03-31T23:59:59Z",
		"month":          "2026-03",
		"namespace":      "team-a",
		"namespace_id":   "ns-1",
		"namespace_path": "team-a/",
		"client_type":    "acme_clients",
	}, 1)
	requireMetricAbsent(t, families, "vault_client_count_monthly_mount_clients", map[string]string{
		"start_time":     "2026-01-01T00:00:00Z",
		"end_time":       "2026-03-31T23:59:59Z",
		"month":          "2026-03",
		"namespace":      "team-a",
		"namespace_id":   "ns-1",
		"namespace_path": "team-a/",
		"mount_path":     "deleted mount; accessor \"auth_approle_deadbeef\"",
		"mount_type":     "deleted mount",
		"client_type":    "clients",
	})
	requireMetricValue(t, families, "vault_client_count_current_namespace_clients", map[string]string{
		"start_time":     "2026-01-01T00:00:00Z",
		"end_time":       "2026-03-31T23:59:59Z",
		"namespace":      "root",
		"namespace_id":   "root",
		"namespace_path": "",
		"client_type":    "entity_clients",
	}, 9)
	requireMetricValue(t, families, "vault_client_count_current_namespace_clients", map[string]string{
		"start_time":     "2026-01-01T00:00:00Z",
		"end_time":       "2026-03-31T23:59:59Z",
		"namespace":      "team-a",
		"namespace_id":   "ns-1",
		"namespace_path": "team-a/",
		"client_type":    "acme_clients",
	}, 1)
	requireMetricValue(t, families, "vault_client_count_current_mount_clients", map[string]string{
		"start_time":     "2026-01-01T00:00:00Z",
		"end_time":       "2026-03-31T23:59:59Z",
		"namespace":      "team-a",
		"namespace_id":   "ns-1",
		"namespace_path": "team-a/",
		"mount_path":     "auth/approle/",
		"mount_type":     "approle",
		"client_type":    "secret_syncs",
	}, 1)
	requireMetricAbsent(t, families, "vault_client_count_current_mount_clients", map[string]string{
		"start_time":     "2026-01-01T00:00:00Z",
		"end_time":       "2026-03-31T23:59:59Z",
		"namespace":      "team-a",
		"namespace_id":   "ns-1",
		"namespace_path": "team-a/",
		"mount_path":     "auth/approle/",
		"mount_type":     "approle",
		"client_type":    "clients",
	})
	require.Nil(t, metricFamilyByName(families, "vault_client_count_namespaces"))
	require.Nil(t, metricFamilyByName(families, "vault_client_count_auth_method"))
	require.Nil(t, metricFamilyByName(families, "vault_client_count_secret_engine"))
}

func TestFailedRefreshKeepsPreviousSnapshotAndMarksStatusFailed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &fakeVaultClient{
		activity: &vault.MonthlyActivityData{
			StartTime: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, time.April, 30, 23, 59, 59, 0, time.UTC),
			ByNamespace: []vault.MonthlyActivityNamespace{
				{
					NamespaceID:   "root",
					NamespacePath: "",
					Counts: vault.ClientCounts{
						Clients:       4,
						EntityClients: 4,
					},
				},
			},
			Months: []vault.MonthlyActivityMonth{
				{
					Timestamp: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
					Counts:    vault.ClientCounts{Clients: 4},
					Namespaces: []vault.MonthlyActivityNamespace{
						{
							NamespaceID:   "root",
							NamespacePath: "",
							Counts:        vault.ClientCounts{Clients: 4},
						},
					},
				},
			},
		},
	}

	c, err := New(
		WithContext(ctx),
		WithTimeout(250*time.Millisecond),
		WithRefreshInterval(time.Hour),
		WithVaultClient(client),
	)
	require.NoError(t, err)

	client.err = fmt.Errorf("boom")
	c.refresh(ctx)

	families := gatherMetricFamilies(t, c)
	requireMetricValue(t, families, "vault_client_count_refresh_success", nil, 0)
	requireMetricValue(t, families, "vault_client_count_activity_period_info", map[string]string{
		"start_time": "2026-01-01T00:00:00Z",
		"end_time":   "2026-04-30T23:59:59Z",
	}, 1)
	requireMetricAbsent(t, families, "vault_client_count_monthly_clients", map[string]string{
		"start_time":  "2026-01-01T00:00:00Z",
		"end_time":    "2026-04-30T23:59:59Z",
		"month":       "2026-04",
		"client_type": "clients",
	})
	requireMetricAbsent(t, families, "vault_client_count_monthly_namespace_clients", map[string]string{
		"start_time":     "2026-01-01T00:00:00Z",
		"end_time":       "2026-04-30T23:59:59Z",
		"month":          "2026-04",
		"namespace":      "root",
		"namespace_id":   "root",
		"namespace_path": "",
		"client_type":    "clients",
	})
	requireMetricValue(t, families, "vault_client_count_current_namespace_clients", map[string]string{
		"start_time":     "2026-01-01T00:00:00Z",
		"end_time":       "2026-04-30T23:59:59Z",
		"namespace":      "root",
		"namespace_id":   "root",
		"namespace_path": "",
		"client_type":    "entity_clients",
	}, 4)
}

func TestNoSuccessfulRefreshEmitsOnlyOperationalMetrics(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &fakeVaultClient{err: fmt.Errorf("unavailable")}

	c, err := New(
		WithContext(ctx),
		WithTimeout(250*time.Millisecond),
		WithRefreshInterval(time.Hour),
		WithBuildInfo("test-version"),
		WithVaultClient(client),
	)
	require.NoError(t, err)

	families := gatherMetricFamilies(t, c)

	require.NotNil(t, metricFamilyByName(families, "vault_client_count_exporter_version"))
	require.NotNil(t, metricFamilyByName(families, "vault_client_count_refresh_success"))
	require.NotNil(t, metricFamilyByName(families, "vault_client_count_refresh_timestamp_seconds"))
	require.NotNil(t, metricFamilyByName(families, "vault_client_count_refresh_duration_seconds"))
	require.Nil(t, metricFamilyByName(families, "vault_client_count_activity_period_info"))
	require.Nil(t, metricFamilyByName(families, "vault_client_count_monthly_clients"))
	require.Nil(t, metricFamilyByName(families, "vault_client_count_monthly_namespace_clients"))
	require.Nil(t, metricFamilyByName(families, "vault_client_count_monthly_mount_clients"))
	require.Nil(t, metricFamilyByName(families, "vault_client_count_current_namespace_clients"))
	require.Nil(t, metricFamilyByName(families, "vault_client_count_current_mount_clients"))
}

func TestEmptyNamespaceAttributionStillEmitsTotals(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &fakeVaultClient{
		activity: &vault.MonthlyActivityData{
			StartTime: time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, time.February, 28, 23, 59, 59, 0, time.UTC),
			Months: []vault.MonthlyActivityMonth{
				{
					Timestamp: time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC),
					Counts: vault.ClientCounts{
						Clients:          9,
						EntityClients:    6,
						NonEntityClients: 3,
					},
				},
			},
		},
	}

	c, err := New(
		WithContext(ctx),
		WithTimeout(250*time.Millisecond),
		WithRefreshInterval(time.Hour),
		WithVaultClient(client),
	)
	require.NoError(t, err)

	families := gatherMetricFamilies(t, c)
	requireMetricValue(t, families, "vault_client_count_activity_period_info", map[string]string{
		"start_time": "2026-02-01T00:00:00Z",
		"end_time":   "2026-02-28T23:59:59Z",
	}, 1)
	requireMetricAbsent(t, families, "vault_client_count_monthly_clients", map[string]string{
		"start_time":  "2026-02-01T00:00:00Z",
		"end_time":    "2026-02-28T23:59:59Z",
		"month":       "2026-02",
		"client_type": "clients",
	})
	requireMetricValue(t, families, "vault_client_count_monthly_clients", map[string]string{
		"start_time":  "2026-02-01T00:00:00Z",
		"end_time":    "2026-02-28T23:59:59Z",
		"month":       "2026-02",
		"client_type": "entity_clients",
	}, 6)
	require.Nil(t, metricFamilyByName(families, "vault_client_count_monthly_namespace_clients"))
	require.Nil(t, metricFamilyByName(families, "vault_client_count_monthly_mount_clients"))
	require.Nil(t, metricFamilyByName(families, "vault_client_count_current_namespace_clients"))
	require.Nil(t, metricFamilyByName(families, "vault_client_count_current_mount_clients"))
}

func gatherMetricFamilies(t *testing.T, collector prometheus.Collector) []*dto.MetricFamily {
	t.Helper()

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)

	families, err := registry.Gather()
	require.NoError(t, err)

	return families
}

func metricFamilyByName(families []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}

	return nil
}

func requireMetricValue(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string, want float64) {
	t.Helper()

	family := metricFamilyByName(families, name)
	require.NotNil(t, family, "metric family %s not found", name)

	for _, metric := range family.Metric {
		if metricLabelsMatch(metric, labels) {
			require.InDelta(t, want, metric.GetGauge().GetValue(), 0.000001)
			return
		}
	}

	t.Fatalf("metric %s with labels %v not found", name, labels)
}

func requireMetricAbsent(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string) {
	t.Helper()

	family := metricFamilyByName(families, name)
	if family == nil {
		return
	}

	for _, metric := range family.Metric {
		if metricLabelsMatch(metric, labels) {
			t.Fatalf("metric %s with labels %v was unexpectedly present", name, labels)
		}
	}
}

func metricLabelsMatch(metric *dto.Metric, want map[string]string) bool {
	if len(want) == 0 {
		return len(metric.Label) == 0
	}

	if len(metric.Label) != len(want) {
		return false
	}

	for _, label := range metric.Label {
		value, ok := want[label.GetName()]
		if !ok || value != label.GetValue() {
			return false
		}
	}

	return true
}
