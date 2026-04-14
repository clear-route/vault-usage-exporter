package vault

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/require"
)

func TestGetMonthlyActivityDecodesNamespacesAndMounts(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/sys/internal/counters/activity/monthly", r.URL.Path)

		_, err := w.Write([]byte(`{
			"data": {
				"clients": 12,
				"entity_clients": 7,
				"non_entity_clients": 3,
				"secret_syncs": 1,
				"acme_clients": 1,
				"by_namespace": [
					{
						"namespace_id": "root",
						"namespace_path": "",
						"counts": {
							"clients": 8,
							"entity_clients": 5,
							"non_entity_clients": 2,
							"secret_syncs": 1,
							"acme_clients": 0
						},
						"mounts": [
							{
								"mount_path": "auth/token/",
								"mount_type": "token/",
								"counts": {
									"clients": 4,
									"entity_clients": 3,
									"non_entity_clients": 1,
									"secret_syncs": 0,
									"acme_clients": 0
								}
							}
						]
					},
					{
						"namespace_id": "ns-123",
						"namespace_path": "platform/team-a/",
						"counts": {
							"clients": 4,
							"entity_clients": 2,
							"non_entity_clients": 1,
							"secret_syncs": 0,
							"acme_clients": 1
						},
						"mounts": [
							{
								"mount_path": "kv/",
								"mount_type": "kv/",
								"counts": {
									"clients": 4,
									"entity_clients": 2,
									"non_entity_clients": 1,
									"secret_syncs": 0,
									"acme_clients": 1
								}
							},
							{
								"mount_path": "auth/userpass/",
								"mount_type": "userpass/",
								"counts": {
									"clients": 1,
									"entity_clients": 1,
									"non_entity_clients": 0,
									"secret_syncs": 0,
									"acme_clients": 0
								}
							}
						]
					}
				]
			}
		}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	activity, err := client.GetMonthlyActivity(context.Background())
	require.NoError(t, err)

	require.Equal(t, ClientCounts{
		Clients:          12,
		EntityClients:    7,
		NonEntityClients: 3,
		SecretSyncs:      1,
		ACMEClients:      1,
	}, activity.ClientCounts)
	require.Len(t, activity.ByNamespace, 2)
	require.Equal(t, "", activity.ByNamespace[0].NamespacePath)
	require.Len(t, activity.ByNamespace[0].Mounts, 1)
	require.Equal(t, "token/", activity.ByNamespace[0].Mounts[0].MountType)
	require.Equal(t, "platform/team-a/", activity.ByNamespace[1].NamespacePath)
	require.Len(t, activity.ByNamespace[1].Mounts, 2)
	require.Equal(t, ClientCounts{
		Clients:          4,
		EntityClients:    2,
		NonEntityClients: 1,
		SecretSyncs:      0,
		ACMEClients:      1,
	}, activity.ByNamespace[1].Counts)
}

func TestGetMonthlyActivityReturnsStatusErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, err := w.Write([]byte(`{"errors":["temporarily unavailable"]}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	activity, err := client.GetMonthlyActivity(context.Background())
	require.Nil(t, activity)
	require.ErrorContains(t, err, "Code: 503")
	require.ErrorContains(t, err, "temporarily unavailable")
}

func newTestClient(t *testing.T, address string) *Client {
	t.Helper()

	apiClient, err := api.NewClient(&api.Config{Address: address})
	require.NoError(t, err)

	return &Client{apiClient: apiClient}
}
