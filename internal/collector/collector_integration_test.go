//go:build integration

package collector

import (
	"context"
	"testing"
	"time"

	"github.com/clear-route/vault-client-count-exporter/pkg/testutils"
	vaultclient "github.com/clear-route/vault-client-count-exporter/pkg/vault"
	"github.com/stretchr/testify/require"
)

func TestCollectorRefreshExposesMonthlyActivityFromVault(t *testing.T) {
	ctx := context.Background()

	container, err := testutils.StartTestContainer(
		"auth enable userpass",
		"write auth/userpass/users/alice password=secret",
	)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, container.Terminate())
	}()

	client, err := vaultclient.NewClientWithToken(container.URI, container.Token)
	require.NoError(t, err)

	_, err = client.Logical().WriteWithContext(ctx, "sys/internal/counters/config", map[string]any{
		"enabled": "enable",
	})
	require.NoError(t, err)

	login, err := client.Logical().WriteWithContext(ctx, "auth/userpass/login/alice", map[string]any{
		"password": "secret",
	})
	require.NoError(t, err)
	require.NotNil(t, login)
	require.NotNil(t, login.Auth)

	userClient, err := vaultclient.NewClientWithToken(container.URI, login.Auth.ClientToken)
	require.NoError(t, err)

	_, err = userClient.Logical().ReadWithContext(ctx, "auth/token/lookup-self")
	require.NoError(t, err)

	_, err = client.Logical().WriteWithContext(ctx, "sys/policies/acl/monthly-reader", map[string]any{
		"policy": `path "sys/internal/counters/activity/monthly" {
  capabilities = ["read"]
}

path "sys/internal/counters/activity" {
  capabilities = ["read"]
}`,
	})
	require.NoError(t, err)

	readonlySecret, err := client.Logical().WriteWithContext(ctx, "auth/token/create", map[string]any{
		"policies": []string{"default", "monthly-reader"},
	})
	require.NoError(t, err)
	require.NotNil(t, readonlySecret)
	require.NotNil(t, readonlySecret.Auth)

	readonlyClient, err := vaultclient.NewClientWithToken(container.URI, readonlySecret.Auth.ClientToken)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		activity, err := client.GetActivity(ctx, vaultclient.ActivityQuery{Monthly: true})
		if err != nil || len(activity.ByNamespace) == 0 {
			return false
		}

		for _, namespace := range activity.ByNamespace {
			if namespace.Counts.Clients > 0 && len(namespace.Mounts) > 0 {
				return true
			}
		}

		return false
	}, 30*time.Second, time.Second)

	refreshCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, err := New(
		WithContext(refreshCtx),
		WithTimeout(5*time.Second),
		WithRefreshInterval(time.Hour),
		WithBuildInfo("integration"),
		WithVaultClient(readonlyClient),
		WithActivityQuery(vaultclient.ActivityQuery{Monthly: true}),
	)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		c.refresh(refreshCtx)

		families := gatherMetricFamilies(t, c)
		totalFamily := metricFamilyByName(families, "vault_client_count_monthly_clients")
		namespaceFamily := metricFamilyByName(families, "vault_client_count_monthly_namespace_clients")
		mountFamily := metricFamilyByName(families, "vault_client_count_monthly_mount_clients")
		currentNamespaceFamily := metricFamilyByName(families, "vault_client_count_current_namespace_clients")
		currentMountFamily := metricFamilyByName(families, "vault_client_count_current_mount_clients")

		return totalFamily != nil && len(totalFamily.Metric) > 0 &&
			namespaceFamily != nil && len(namespaceFamily.Metric) > 0 &&
			mountFamily != nil && len(mountFamily.Metric) > 0 &&
			currentNamespaceFamily != nil && len(currentNamespaceFamily.Metric) > 0 &&
			currentMountFamily != nil && len(currentMountFamily.Metric) > 0
	}, 30*time.Second, time.Second)
}
