package vault

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/require"
)

func TestGetActivityDecodesMonthlyEndpointNamespacesAndMounts(t *testing.T) {
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
				"start_time": "2026-01-01T00:00:00Z",
				"end_time": "2026-03-31T23:59:59Z",
				"months": [
					{
						"timestamp": "2026-03-01T00:00:00Z",
						"counts": {
							"clients": 12,
							"entity_clients": 7,
							"non_entity_clients": 3,
							"secret_syncs": 1,
							"acme_clients": 1
						},
						"namespaces": [
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
							}
						]
					}
				],
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

	activity, err := client.GetActivity(context.Background(), ActivityQuery{
		StartTime: "2026-01-01T00:00:00Z",
		EndTime:   "2026-03-31T23:59:59Z",
		Monthly:   true,
	})
	require.NoError(t, err)

	require.Equal(t, ClientCounts{
		Clients:          12,
		EntityClients:    7,
		NonEntityClients: 3,
		SecretSyncs:      1,
		ACMEClients:      1,
	}, activity.ClientCounts)
	require.Equal(t, "2026-01-01T00:00:00Z", activity.StartTime.Format(time.RFC3339))
	require.Equal(t, "2026-03-31T23:59:59Z", activity.EndTime.Format(time.RFC3339))
	require.Len(t, activity.Months, 1)
	require.Equal(t, "2026-03-01T00:00:00Z", activity.Months[0].Timestamp.Format(time.RFC3339))
	require.Equal(t, ClientCounts{
		Clients:          12,
		EntityClients:    7,
		NonEntityClients: 3,
		SecretSyncs:      1,
		ACMEClients:      1,
	}, activity.Months[0].Counts)
	require.Len(t, activity.Months[0].Namespaces, 1)
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

func TestGetMonthlyActivityPreservesDeletedMountPayload(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/sys/internal/counters/activity/monthly", r.URL.Path)

		_, err := w.Write([]byte(`{
			"data": {
				"clients": 3,
				"entity_clients": 3,
				"non_entity_clients": 0,
				"secret_syncs": 0,
				"acme_clients": 0,
				"by_namespace": [
					{
						"namespace_id": "root",
						"namespace_path": "",
						"counts": {
							"clients": 3,
							"entity_clients": 3,
							"non_entity_clients": 0,
							"secret_syncs": 0,
							"acme_clients": 0
						},
						"mounts": [
							{
								"mount_path": "deleted mount; accessor \"auth_approle_deadbeef\"",
								"mount_type": "deleted mount",
								"counts": {
									"clients": 3,
									"entity_clients": 3,
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

	activity, err := client.GetActivity(context.Background(), ActivityQuery{Monthly: true})
	require.NoError(t, err)
	require.Len(t, activity.ByNamespace, 1)
	require.Len(t, activity.ByNamespace[0].Mounts, 1)
	require.Equal(t, "deleted mount; accessor \"auth_approle_deadbeef\"", activity.ByNamespace[0].Mounts[0].MountPath)
	require.Equal(t, "deleted mount", activity.ByNamespace[0].Mounts[0].MountType)
}

func TestGetActivityReturnsStatusErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, err := w.Write([]byte(`{"errors":["temporarily unavailable"]}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	activity, err := client.GetActivity(context.Background(), ActivityQuery{Monthly: true})
	require.Nil(t, activity)
	require.ErrorContains(t, err, "503")
	require.ErrorContains(t, err, "temporarily unavailable")
}

func TestGetActivityNormalizesCumulativeEndpointTotalsAndQueryParams(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/sys/internal/counters/activity", r.URL.Path)
		require.Equal(t, "2025-01-15T11:22:33Z", r.URL.Query().Get("start_time"))
		require.Equal(t, "2025-03-20T08:09:10Z", r.URL.Query().Get("end_time"))

		_, err := w.Write([]byte(`{
			"data": {
				"start_time": "2025-01-01T00:00:00Z",
				"end_time": "2025-03-31T23:59:59Z",
				"total": {
					"clients": 10,
					"entity_clients": 6,
					"non_entity_clients": 2,
					"secret_syncs": 1,
					"acme_clients": 1
				},
				"months": [
					{
						"timestamp": "2025-03-01T00:00:00Z",
						"counts": {
							"clients": 4,
							"entity_clients": 2,
							"non_entity_clients": 1,
							"secret_syncs": 1,
							"acme_clients": 0
						},
						"namespaces": [
							{
								"namespace_id": "root",
								"namespace_path": "",
								"counts": {
									"clients": 4,
									"entity_clients": 2,
									"non_entity_clients": 1,
									"secret_syncs": 1,
									"acme_clients": 0
								},
								"mounts": [
									{
										"mount_path": "auth/token/",
										"mount_type": "token/",
										"counts": {
											"clients": 4,
											"entity_clients": 2,
											"non_entity_clients": 1,
											"secret_syncs": 1,
											"acme_clients": 0
										}
									}
								]
							}
						]
					}
				],
				"by_namespace": [
					{
						"namespace_id": "root",
						"namespace_path": "",
						"counts": {
							"clients": 10,
							"entity_clients": 6,
							"non_entity_clients": 2,
							"secret_syncs": 1,
							"acme_clients": 1
						},
						"mounts": [
							{
								"mount_path": "auth/token/",
								"mount_type": "token/",
								"counts": {
									"clients": 10,
									"entity_clients": 6,
									"non_entity_clients": 2,
									"secret_syncs": 1,
									"acme_clients": 1
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

	activity, err := client.GetActivity(context.Background(), ActivityQuery{
		StartTime: "2025-01-15T11:22:33Z",
		EndTime:   "2025-03-20T08:09:10Z",
	})
	require.NoError(t, err)
	require.Equal(t, ClientCounts{
		Clients:          10,
		EntityClients:    6,
		NonEntityClients: 2,
		SecretSyncs:      1,
		ACMEClients:      1,
	}, activity.ClientCounts)
	require.Equal(t, "2025-01-01T00:00:00Z", activity.StartTime.Format(time.RFC3339))
	require.Equal(t, "2025-03-31T23:59:59Z", activity.EndTime.Format(time.RFC3339))
	require.Len(t, activity.Months, 1)
	require.Len(t, activity.ByNamespace, 1)
}

func TestGetActivityReadsFixtureFile(t *testing.T) {
	t.Parallel()

	fixture, err := os.CreateTemp(t.TempDir(), "monthly-activity-*.json")
	require.NoError(t, err)

	_, err = fixture.WriteString(`{
		"data": {
			"clients": 9,
			"entity_clients": 6,
			"non_entity_clients": 2,
			"secret_syncs": 1,
			"acme_clients": 0,
			"by_namespace": [
				{
					"namespace_id": "root",
					"namespace_path": "",
					"counts": {
						"clients": 9,
						"entity_clients": 6,
						"non_entity_clients": 2,
						"secret_syncs": 1,
						"acme_clients": 0
					},
					"mounts": [
						{
							"mount_path": "auth/token/",
							"mount_type": "token/",
							"counts": {
								"clients": 9,
								"entity_clients": 6,
								"non_entity_clients": 2,
								"secret_syncs": 1,
								"acme_clients": 0
							}
						}
					]
				}
			]
		}
	}`)
	require.NoError(t, err)
	require.NoError(t, fixture.Close())

	client := &Client{fixturePath: fixture.Name()}

	activity, err := client.GetActivity(context.Background(), ActivityQuery{})
	require.NoError(t, err)
	require.Equal(t, 9, activity.Clients)
	require.Len(t, activity.ByNamespace, 1)
	require.Equal(t, "auth/token/", activity.ByNamespace[0].Mounts[0].MountPath)
}

func TestNewUsesFixtureFileWithoutVaultAddr(t *testing.T) {
	fixture, err := os.CreateTemp(t.TempDir(), "monthly-activity-*.json")
	require.NoError(t, err)
	require.NoError(t, fixture.Close())

	t.Setenv(fixturePathEnv, fixture.Name())
	t.Setenv("VAULT_ADDR", "")

	client, err := New()
	require.NoError(t, err)
	require.Equal(t, fixture.Name(), client.fixturePath)
	require.Nil(t, client.apiClient)
}

func TestNewRejectsEmptyFixturePath(t *testing.T) {
	t.Setenv(fixturePathEnv, "")

	client, err := New()
	require.Nil(t, client)
	require.ErrorContains(t, err, fixturePathEnv)
}

func newTestClient(t *testing.T, address string) *Client {
	t.Helper()

	apiClient, err := api.NewClient(&api.Config{Address: address})
	require.NoError(t, err)

	return &Client{apiClient: apiClient}
}
