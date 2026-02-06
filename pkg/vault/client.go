package vault

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/vault/api"
)

// Client represents a vault struct used for reading and writing secrets.
type Client struct {
	Client *api.Client
}

// Engine represents a vault auth method or secret engine.
type Engine struct {
	Name      string
	Type      string
	Path      string
	Namespace string
}

// New returns a new vault client wrapper.
func New() (*Client, error) {
	addr, ok := os.LookupEnv("VAULT_ADDR")
	if !ok {
		return nil, fmt.Errorf("vault address not set in VAULT_ADDR environment variable")
	}

	cfg := api.DefaultConfig()
	cfg.Address = addr

	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create vault client: %w", err)
	}

	return &Client{Client: client}, nil
}

// NewClientWithToken returns a new vault client wrapper.
func NewClientWithToken(addr, token string) (*Client, error) {
	cfg := &api.Config{
		Address: addr,
	}

	c, err := api.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	c.SetToken(token)

	return &Client{Client: c}, nil
}

// ListNamespaces returns a list of namespaces in the vault. If namespaces are not supported, it returns an empty list.
func (c *Client) ListNamespaces(ctx context.Context) ([]string, error) {
	secret, err := c.Client.Logical().ListWithContext(ctx, "sys/namespaces")
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
	if namespace != "" {
		c.Client.SetNamespace(namespace)
		defer c.Client.SetNamespace("")
	}

	mounts, err := c.Client.Sys().ListAuthWithContext(ctx)
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

func (c *Client) ListSecretEngines(ctx context.Context, namespace string) ([]Engine, error) {
	if namespace != "" {
		c.Client.SetNamespace(namespace)
		defer c.Client.SetNamespace("")
	}

	mounts, err := c.Client.Sys().ListMountsWithContext(ctx)
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

func (c *Client) CountLeases(ctx context.Context, namespace string) (int, error) {
	if namespace != "" {
		c.Client.SetNamespace(namespace)
		defer c.Client.SetNamespace("")
	}

	visited := map[string]struct{}{}
	queue := []string{""}
	count := 0

	for len(queue) > 0 {
		prefix := queue[0]
		queue = queue[1:]

		if _, ok := visited[prefix]; ok {
			continue
		}
		visited[prefix] = struct{}{}

		secret, err := c.Client.Logical().ListWithContext(ctx, "sys/leases/lookup/"+prefix)
		if err != nil {
			return 0, fmt.Errorf("list leases for prefix %q: %w", prefix, err)
		}
		if secret == nil || secret.Data == nil {
			continue
		}

		keys, _ := secret.Data["keys"].([]interface{})
		for _, k := range keys {
			s, ok := k.(string)
			if !ok {
				continue
			}
			if strings.HasSuffix(s, "/") {
				queue = append(queue, prefix+s)
				continue
			}
			count++
		}
	}

	return count, nil
}

func (c *Client) CountTokens(ctx context.Context, namespace string) (int, error) {
	if namespace != "" {
		c.Client.SetNamespace(namespace)
		defer c.Client.SetNamespace("")
	}

	secret, err := c.Client.Logical().ListWithContext(ctx, "auth/token/accessors")
	if err != nil {
		return 0, fmt.Errorf("list token accessors: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return 0, nil
	}

	keys, _ := secret.Data["keys"].([]interface{})
	return len(keys), nil
}
