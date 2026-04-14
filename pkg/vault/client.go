package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/hashicorp/vault/api"
)

// Client represents a vault struct used for reading and writing secrets.
type Client struct {
	apiClient *api.Client
}

// Engine represents a vault auth method or secret engine.
type Engine struct {
	Name      string
	Type      string
	Path      string
	Namespace string
}

// ClientCounts models the client activity counters returned by Vault.
type ClientCounts struct {
	Clients          int `json:"clients"`
	EntityClients    int `json:"entity_clients"`
	NonEntityClients int `json:"non_entity_clients"`
	SecretSyncs      int `json:"secret_syncs"`
	ACMEClients      int `json:"acme_clients"`
}

// MonthlyActivityMount is the mount-level attribution from the monthly activity API.
type MonthlyActivityMount struct {
	MountPath string       `json:"mount_path"`
	MountType string       `json:"mount_type"`
	Counts    ClientCounts `json:"counts"`
}

// MonthlyActivityNamespace is the namespace-level attribution from the monthly activity API.
type MonthlyActivityNamespace struct {
	NamespaceID   string                 `json:"namespace_id"`
	NamespacePath string                 `json:"namespace_path"`
	Counts        ClientCounts           `json:"counts"`
	Mounts        []MonthlyActivityMount `json:"mounts"`
}

// MonthlyActivityData is the subset of the partial-month activity response used by the exporter.
type MonthlyActivityData struct {
	ClientCounts
	ByNamespace []MonthlyActivityNamespace `json:"by_namespace"`
}

type monthlyActivityResponse struct {
	Data MonthlyActivityData `json:"data"`
}

// New returns a new vault client wrapper.
func New() (*Client, error) {
	addr, ok := os.LookupEnv("VAULT_ADDR")
	if !ok {
		return nil, fmt.Errorf("vault address not set in VAULT_ADDR")
	}

	cfg := api.DefaultConfig()
	cfg.Address = addr

	apiClient, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create vault client: %w", err)
	}

	return &Client{apiClient: apiClient}, nil
}

// NewClientWithToken returns a new vault client wrapper.
func NewClientWithToken(addr, token string) (*Client, error) {
	apiClient, err := api.NewClient(&api.Config{Address: addr})
	if err != nil {
		return nil, fmt.Errorf("create vault client: %w", err)
	}

	apiClient.SetToken(token)

	return &Client{apiClient: apiClient}, nil
}

func (c *Client) Sys() *api.Sys {
	return c.apiClient.Sys()
}

func (c *Client) Logical() *api.Logical {
	return c.apiClient.Logical()
}

// ListNamespaces returns a list of namespaces in the vault. If namespaces are not supported, it returns an empty list.
func (c *Client) ListNamespaces(ctx context.Context) ([]string, error) {
	secret, err := c.Logical().ListWithContext(ctx, "sys/namespaces")
	if err != nil {
		return nil, fmt.Errorf("error listing namespaces: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return []string{}, nil
	}

	keys, _ := secret.Data["keys"].([]interface{})
	if len(keys) == 0 {
		return []string{}, nil
	}

	namespaces := make([]string, 0, len(keys))

	for _, k := range keys {
		s, ok := k.(string)
		if !ok {
			continue
		}

		ns := strings.TrimSuffix(s, "/")
		if ns == "" {
			continue
		}

		namespaces = append(namespaces, ns)
	}

	return namespaces, nil
}

func (c *Client) ListAuthMethods(ctx context.Context, namespace string) ([]Engine, error) {
	restoreNamespace := c.setNamespace(namespace)
	defer restoreNamespace()

	mounts, err := c.Sys().ListAuthWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list auth methods: %w", err)
	}

	out := make([]Engine, 0, len(mounts))

	for path, m := range mounts {
		name := strings.TrimSuffix(path, "/")
		out = append(out, Engine{
			Name:      name,
			Type:      m.Type,
			Path:      path,
			Namespace: namespace,
		})
	}

	return out, nil
}

// GetMonthlyActivity fetches the current-month activity snapshot from Vault.
func (c *Client) GetMonthlyActivity(ctx context.Context) (*MonthlyActivityData, error) {
	restoreNamespace := c.setNamespace("")
	defer restoreNamespace()

	resp, err := c.apiClient.Logical().ReadRawWithContext(ctx, "sys/internal/counters/activity/monthly")
	if err != nil {
		return nil, fmt.Errorf("get monthly activity: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		if readErr != nil {
			return nil, fmt.Errorf("get monthly activity: unexpected status %d and failed to read body: %w", resp.StatusCode, readErr)
		}

		return nil, fmt.Errorf("get monthly activity: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded monthlyActivityResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode monthly activity response: %w", err)
	}

	return &decoded.Data, nil
}

func (c *Client) ListSecretEngines(ctx context.Context, namespace string) ([]Engine, error) {
	restoreNamespace := c.setNamespace(namespace)
	defer restoreNamespace()

	mounts, err := c.Sys().ListMountsWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list secret engines: %w", err)
	}

	out := make([]Engine, 0, len(mounts))

	for path, m := range mounts {
		name := strings.TrimSuffix(path, "/")
		out = append(out, Engine{
			Name:      name,
			Type:      m.Type,
			Path:      path,
			Namespace: namespace,
		})
	}

	return out, nil
}

func (c *Client) setNamespace(namespace string) func() {
	if namespace == "" || namespace == "root" {
		namespace = ""
	}

	c.apiClient.SetNamespace(namespace)

	return func() {
		c.apiClient.SetNamespace("")
	}
}
