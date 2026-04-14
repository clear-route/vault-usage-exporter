package collector

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/clear-route/vault-usage-exporter/pkg/vault"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

type fakeVaultClient struct {
	namespaces []string
	auth       map[string][]vault.Engine
	engines    map[string][]vault.Engine
	activity   *vault.MonthlyActivityData

	err error

	listNamespacesCalls int
	listAuthCalls       int
	listSecretCalls     int
	getActivityCalls    int
}

func (f *fakeVaultClient) ListNamespaces(context.Context) ([]string, error) {
	f.listNamespacesCalls++
	if f.err != nil {
		return nil, f.err
	}

	return append([]string(nil), f.namespaces...), nil
}

func (f *fakeVaultClient) ListAuthMethods(_ context.Context, namespace string) ([]vault.Engine, error) {
	f.listAuthCalls++
	if f.err != nil {
		return nil, f.err
	}

	return append([]vault.Engine(nil), f.auth[namespace]...), nil
}

func (f *fakeVaultClient) ListSecretEngines(_ context.Context, namespace string) ([]vault.Engine, error) {
	f.listSecretCalls++
	if f.err != nil {
		return nil, f.err
	}

	return append([]vault.Engine(nil), f.engines[namespace]...), nil
}

func (f *fakeVaultClient) GetMonthlyActivity(context.Context) (*vault.MonthlyActivityData, error) {
	f.getActivityCalls++
	if f.err != nil {
		return nil, f.err
	}

	if f.activity == nil {
		return &vault.MonthlyActivityData{}, nil
	}

	copyValue := *f.activity
	copyValue.ByNamespace = append([]vault.MonthlyActivityNamespace(nil), f.activity.ByNamespace...)

	return &copyValue, nil
}

func TestCollectUsesCachedSnapshotAndEmitsNamespaceAndMountMetrics(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &fakeVaultClient{
		namespaces: []string{"team-a"},
		auth: map[string][]vault.Engine{
			"root": {
				{Name: "token", Type: "token", Path: "token/", Namespace: "root"},
			},
			"team-a": {
				{Name: "userpass", Type: "userpass", Path: "userpass/", Namespace: "team-a"},
			},
		},
		engines: map[string][]vault.Engine{
			"root": {
				{Name: "sys", Type: "system", Path: "sys/", Namespace: "root"},
			},
			"team-a": {
				{Name: "kv", Type: "kv", Path: "kv/", Namespace: "team-a"},
			},
		},
		activity: &vault.MonthlyActivityData{
			ClientCounts: vault.ClientCounts{
				Clients:          11,
				EntityClients:    7,
				NonEntityClients: 2,
				SecretSyncs:      1,
				ACMEClients:      1,
			},
			ByNamespace: []vault.MonthlyActivityNamespace{
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
							MountPath: "kv/",
							MountType: "kv/",
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
	}

	c, err := New(
		WithContext(ctx),
		WithTimeout(250*time.Millisecond),
		WithRefreshInterval(time.Hour),
		WithBuildInfo("test-version"),
		WithVaultClient(client),
	)
	require.NoError(t, err)

	initialCalls := []int{
		client.listNamespacesCalls,
		client.listAuthCalls,
		client.listSecretCalls,
		client.getActivityCalls,
	}

	families := gatherMetricFamilies(t, c)

	require.Equal(t, initialCalls, []int{
		client.listNamespacesCalls,
		client.listAuthCalls,
		client.listSecretCalls,
		client.getActivityCalls,
	}, "Collect should only read cached state")

	requireMetricValue(t, families, "vault_usage_refresh_success", nil, 1)
	requireMetricValue(t, families, "vault_usage_namespace_clients", map[string]string{
		"namespace":      "root",
		"namespace_id":   "root",
		"namespace_path": "",
		"client_type":    "clients",
	}, 5)
	requireMetricValue(t, families, "vault_usage_namespace_clients", map[string]string{
		"namespace":      "team-a",
		"namespace_id":   "ns-1",
		"namespace_path": "team-a/",
		"client_type":    "acme_clients",
	}, 1)
	requireMetricValue(t, families, "vault_usage_mount_clients", map[string]string{
		"namespace":      "team-a",
		"namespace_id":   "ns-1",
		"namespace_path": "team-a/",
		"mount_path":     "kv/",
		"mount_type":     "kv",
		"client_type":    "clients",
	}, 6)
	requireMetricValue(t, families, "vault_usage_namespaces", map[string]string{"name": "root"}, 1)
	requireMetricValue(t, families, "vault_usage_namespaces", map[string]string{"name": "team-a"}, 1)
	requireMetricValue(t, families, "vault_usage_auth_method", map[string]string{
		"name":      "userpass",
		"type":      "userpass",
		"path":      "userpass/",
		"namespace": "team-a",
	}, 1)
	requireMetricValue(t, families, "vault_usage_secret_engine", map[string]string{
		"name":      "kv",
		"type":      "kv",
		"path":      "kv/",
		"namespace": "team-a",
	}, 1)
	require.Nil(t, metricFamilyByName(families, "vault_usage_months"))
	require.Nil(t, metricFamilyByName(families, "vault_usage_new_clients"))
}

func TestFailedRefreshKeepsPreviousSnapshotAndMarksStatusFailed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &fakeVaultClient{
		activity: &vault.MonthlyActivityData{
			ByNamespace: []vault.MonthlyActivityNamespace{
				{
					NamespaceID:   "root",
					NamespacePath: "",
					Counts:        vault.ClientCounts{Clients: 4},
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

	requireMetricValue(t, families, "vault_usage_refresh_success", nil, 0)
	requireMetricValue(t, families, "vault_usage_namespace_clients", map[string]string{
		"namespace":      "root",
		"namespace_id":   "root",
		"namespace_path": "",
		"client_type":    "clients",
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

	require.NotNil(t, metricFamilyByName(families, "vault_usage_exporter_version"))
	require.NotNil(t, metricFamilyByName(families, "vault_usage_refresh_success"))
	require.NotNil(t, metricFamilyByName(families, "vault_usage_refresh_timestamp_seconds"))
	require.NotNil(t, metricFamilyByName(families, "vault_usage_refresh_duration_seconds"))
	require.Nil(t, metricFamilyByName(families, "vault_usage_namespaces"))
	require.Nil(t, metricFamilyByName(families, "vault_usage_auth_method"))
	require.Nil(t, metricFamilyByName(families, "vault_usage_secret_engine"))
	require.Nil(t, metricFamilyByName(families, "vault_usage_namespace_clients"))
	require.Nil(t, metricFamilyByName(families, "vault_usage_mount_clients"))
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
