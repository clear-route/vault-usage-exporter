package exporter

// import (
// 	"context"
// 	"fmt"
// 	"testing"
// 	"time"

// 	"github.com/clear-route/vault-usage-reporter/pkg/vault"
// 	vaultapi "github.com/hashicorp/vault/api"
// 	"github.com/prometheus/client_golang/prometheus"
// 	"github.com/stretchr/testify/require"
// 	"github.com/testcontainers/testcontainers-go"
// 	"github.com/testcontainers/testcontainers-go/wait"
// )

// func TestCollector_WithRealVault(t *testing.T) {
// 	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
// 	defer cancel()

// 	req := testcontainers.ContainerRequest{
// 		Image:        "hashicorp/vault:1.16.2",
// 		ExposedPorts: []string{"8200/tcp"},
// 		Cmd: []string{
// 			"server",
// 			"-dev",
// 			"-dev-root-token-id=root",
// 			"-dev-listen-address=0.0.0.0:8200",
// 		},
// 		WaitingFor: wait.ForHTTP("/v1/sys/health").WithPort("8200/tcp").WithStartupTimeout(60 * time.Second),
// 	}

// 	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
// 		ContainerRequest: req,
// 		Started:          true,
// 	})
// 	require.NoError(t, err)
// 	t.Cleanup(func() {
// 		_ = container.Terminate(context.Background())
// 	})

// 	host, err := container.Host(ctx)
// 	require.NoError(t, err)

// 	port, err := container.MappedPort(ctx, "8200/tcp")
// 	require.NoError(t, err)

// 	addr := fmt.Sprintf("http://%s:%s", host, port.Port())

// 	vcfg := vaultapi.DefaultConfig()
// 	vcfg.Address = addr
// 	apiClient, err := vaultapi.NewClient(vcfg)
// 	require.NoError(t, err)
// 	apiClient.SetToken("root")

// 	_ = apiClient.Sys().EnableAuthWithOptions("userpass", &vaultapi.EnableAuthOptions{Type: "userpass"})
// 	_ = apiClient.Sys().Mount("kv", &vaultapi.MountInput{Type: "kv"})

// 	t.Setenv("VAULT_TOKEN", "root")

// 	vaultClient, err := vault.New(addr)
// 	require.NoError(t, err)

// 	collector := NewCollector(ctx, vaultClient, Options{
// 		CollectAuthMethods:   true,
// 		CollectSecretEngines: true,
// 		Timeout:              10 * time.Second,
// 	})

// 	reg := prometheus.NewRegistry()
// 	reg.MustRegister(collector)

// 	mfs, err := reg.Gather()
// 	require.NoError(t, err)

// 	requireMetricWithLabels(t, mfs, "vault_usage_auth_method", map[string]string{
// 		"name":      "userpass",
// 		"type":      "userpass",
// 		"path":      "userpass/",
// 		"namespace": "root",
// 	})

// 	requireMetricWithLabels(t, mfs, "vault_usage_secret_engine", map[string]string{
// 		"name":      "kv",
// 		"type":      "kv",
// 		"path":      "kv/",
// 		"namespace": "root",
// 	})
// }
