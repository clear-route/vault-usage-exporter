package exporter

// import (
// 	"context"
// 	"testing"
// 	"time"

// 	"github.com/clear-route/vault-usage-reporter/pkg/vault"
// 	"github.com/prometheus/client_golang/prometheus"
// 	dto "github.com/prometheus/client_model/go"
// 	"github.com/stretchr/testify/require"
// )

// type fakeVaultClient struct {
// 	namespaces    []string
// 	authMethods   map[string][]vault.AuthMethod
// 	secretEngines map[string][]vault.SecretEngine
// 	leases        map[string]int
// 	tokens        map[string]int
// }

// func (f *fakeVaultClient) ListNamespaces(ctx context.Context) ([]string, error) {
// 	_ = ctx
// 	return f.namespaces, nil
// }

// func (f *fakeVaultClient) ListAuthMethods(ctx context.Context, namespace string) ([]vault.AuthMethod, error) {
// 	_ = ctx
// 	return f.authMethods[namespace], nil
// }

// func (f *fakeVaultClient) ListSecretEngines(ctx context.Context, namespace string) ([]vault.SecretEngine, error) {
// 	_ = ctx
// 	return f.secretEngines[namespace], nil
// }

// func (f *fakeVaultClient) CountLeases(ctx context.Context, namespace string) (int, error) {
// 	_ = ctx
// 	return f.leases[namespace], nil
// }

// func (f *fakeVaultClient) CountTokens(ctx context.Context, namespace string) (int, error) {
// 	_ = ctx
// 	return f.tokens[namespace], nil
// }

// func TestCollector_Gather(t *testing.T) {
// 	t.Parallel()

// 	client := &fakeVaultClient{
// 		namespaces: []string{"root", "team-a"},
// 		authMethods: map[string][]vault.AuthMethod{
// 			"root": {
// 				{Name: "token", Type: "token", Path: "token/", Namespace: "root"},
// 			},
// 			"team-a": {
// 				{Name: "userpass", Type: "userpass", Path: "userpass/", Namespace: "team-a"},
// 			},
// 		},
// 		secretEngines: map[string][]vault.SecretEngine{
// 			"root": {
// 				{Name: "kv", Type: "kv", Path: "kv/", Namespace: "root"},
// 			},
// 			"team-a": {
// 				{Name: "transit", Type: "transit", Path: "transit/", Namespace: "team-a"},
// 			},
// 		},
// 		leases: map[string]int{"root": 3, "team-a": 0},
// 		tokens: map[string]int{"root": 2, "team-a": 1},
// 	}

// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	collector := NewCollector(ctx, client, Options{
// 		CollectAuthMethods:   true,
// 		CollectSecretEngines: true,
// 		Timeout:              2 * time.Second,
// 	})

// 	reg := prometheus.NewRegistry()
// 	reg.MustRegister(collector)

// 	mfs, err := reg.Gather()
// 	require.NoError(t, err)

// 	requireMetricWithLabelValue(t, mfs, "vault_usage_namespace", "name", "root")
// 	requireMetricWithLabelValue(t, mfs, "vault_usage_namespace", "name", "team-a")

// 	requireMetricWithLabels(t, mfs, "vault_usage_auth_method", map[string]string{
// 		"name":      "token",
// 		"type":      "token",
// 		"path":      "token/",
// 		"namespace": "root",
// 	})
// 	requireMetricWithLabels(t, mfs, "vault_usage_secret_engine", map[string]string{
// 		"name":      "transit",
// 		"type":      "transit",
// 		"path":      "transit/",
// 		"namespace": "team-a",
// 	})

// 	requireMetricWithLabels(t, mfs, "vault_usage_leases", map[string]string{"namespace": "root"})
// 	requireMetricWithLabels(t, mfs, "vault_usage_tokens", map[string]string{"namespace": "team-a"})
// }

// func requireMetricWithLabelValue(t *testing.T, mfs []*dto.MetricFamily, metricName, labelName, labelValue string) {
// 	requireMetricWithLabels(t, mfs, metricName, map[string]string{labelName: labelValue})
// }

// func requireMetricWithLabels(t *testing.T, mfs []*dto.MetricFamily, metricName string, labels map[string]string) {
// 	t.Helper()

// 	var target *dto.MetricFamily
// 	for _, mf := range mfs {
// 		if mf.GetName() == metricName {
// 			target = mf
// 			break
// 		}
// 	}
// 	require.NotNil(t, target, "metric %s not found", metricName)

// 	for _, m := range target.GetMetric() {
// 		labelMap := map[string]string{}
// 		for _, lp := range m.GetLabel() {
// 			labelMap[lp.GetName()] = lp.GetValue()
// 		}

// 		match := true
// 		for k, v := range labels {
// 			if labelMap[k] != v {
// 				match = false
// 				break
// 			}
// 		}
// 		if match {
// 			return
// 		}
// 	}

// 	require.Fail(t, "metric not found with labels", "%s labels=%v", metricName, labels)
// }
