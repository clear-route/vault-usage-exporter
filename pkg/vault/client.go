package vault

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/vault/api"
)

// Client represents a vault struct used for reading and writing secrets.
type Client struct {
	Client *api.Client
}

type AuthMethod struct {
	Name      string
	Type      string
	Path      string
	Namespace string
}

type SecretEngine struct {
	Name      string
	Type      string
	Path      string
	Namespace string
}

func New(address string) (*Client, error) {
	cfg := api.DefaultConfig()
	cfg.Address = address

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

func (c *Client) ListNamespaces(ctx context.Context) ([]string, error) {
	secret, err := c.Client.Logical().ListWithContext(ctx, "sys/namespaces")
	if err != nil {
		if isNotFound(err) {
			return []string{"root"}, nil
		}
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	if secret == nil || secret.Data == nil {
		return []string{"root"}, nil
	}

	keys, _ := secret.Data["keys"].([]interface{})
	if len(keys) == 0 {
		return []string{"root"}, nil
	}

	namespaces := make([]string, 0, len(keys)+1)
	namespaces = append(namespaces, "root")
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

func (c *Client) ListAuthMethods(ctx context.Context, namespace string) ([]AuthMethod, error) {
	client, err := c.clientForNamespace(namespace)
	if err != nil {
		return nil, err
	}

	mounts, err := client.Sys().ListAuthWithContext(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list auth methods: %w", err)
	}

	out := make([]AuthMethod, 0, len(mounts))
	for path, m := range mounts {
		name := strings.TrimSuffix(path, "/")
		out = append(out, AuthMethod{
			Name:      name,
			Type:      m.Type,
			Path:      path,
			Namespace: namespaceOrRoot(namespace),
		})
	}

	return out, nil
}

func (c *Client) ListSecretEngines(ctx context.Context, namespace string) ([]SecretEngine, error) {
	client, err := c.clientForNamespace(namespace)
	if err != nil {
		return nil, err
	}

	mounts, err := client.Sys().ListMountsWithContext(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list secret engines: %w", err)
	}

	out := make([]SecretEngine, 0, len(mounts))
	for path, m := range mounts {
		name := strings.TrimSuffix(path, "/")
		out = append(out, SecretEngine{
			Name:      name,
			Type:      m.Type,
			Path:      path,
			Namespace: namespaceOrRoot(namespace),
		})
	}

	return out, nil
}

func (c *Client) CountLeases(ctx context.Context, namespace string) (int, error) {
	client, err := c.clientForNamespace(namespace)
	if err != nil {
		return 0, err
	}

	const basePath = "sys/leases/lookup/"

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

		secret, err := client.Logical().ListWithContext(ctx, basePath+prefix)
		if err != nil {
			if isNotFound(err) || isForbidden(err) {
				return 0, nil
			}
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
	client, err := c.clientForNamespace(namespace)
	if err != nil {
		return 0, err
	}

	secret, err := client.Logical().ListWithContext(ctx, "auth/token/accessors")
	if err != nil {
		if isNotFound(err) || isForbidden(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("list token accessors: %w", err)
	}
	if secret == nil || secret.Data == nil {
		return 0, nil
	}

	keys, _ := secret.Data["keys"].([]interface{})
	return len(keys), nil
}

func (c *Client) clientForNamespace(namespace string) (*api.Client, error) {
	if namespace == "" || namespace == "root" {
		return c.Client, nil
	}

	cloned, err := c.Client.Clone()
	if err != nil {
		return nil, fmt.Errorf("clone client: %w", err)
	}
	cloned.SetNamespace(namespace)
	return cloned, nil
}

func namespaceOrRoot(namespace string) string {
	if namespace == "" {
		return "root"
	}
	return namespace
}

func statusCode(err error) (int, bool) {
	var respErr *api.ResponseError
	if errors.As(err, &respErr) {
		return respErr.StatusCode, true
	}
	return 0, false
}

func isNotFound(err error) bool {
	code, ok := statusCode(err)
	return ok && code == 404
}

func isForbidden(err error) bool {
	code, ok := statusCode(err)
	return ok && code == 403
}
