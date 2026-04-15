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

const fixturePathEnv = "VAULT_MONTHLY_ACTIVITY_FILE"

// Client represents a vault struct used for reading and writing secrets.
type Client struct {
	apiClient   *api.Client
	fixturePath string
}

// New returns a new vault client wrapper.
func New() (*Client, error) {
	if fixturePath, ok := os.LookupEnv(fixturePathEnv); ok {
		if fixturePath == "" {
			return nil, fmt.Errorf("%s is set but empty", fixturePathEnv)
		}

		return &Client{fixturePath: fixturePath}, nil
	}

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

// Logical returns the underlying logical client for integration setup.
func (c *Client) Logical() *api.Logical {
	return c.apiClient.Logical()
}

// Sys returns the underlying sys client for integration setup.
func (c *Client) Sys() *api.Sys {
	return c.apiClient.Sys()
}

// GetActivity fetches activity data from Vault and normalizes the response for
// the selected endpoint.
func (c *Client) GetActivity(ctx context.Context, query ActivityQuery) (*MonthlyActivityData, error) {
	if c.fixturePath != "" {
		file, err := os.Open(c.fixturePath)
		if err != nil {
			return nil, fmt.Errorf("open activity fixture %q: %w", c.fixturePath, err)
		}
		defer file.Close()

		activity, err := decodeActivity(file)
		if err != nil {
			return nil, fmt.Errorf("decode activity fixture %q: %w", c.fixturePath, err)
		}

		return activity, nil
	}

	params := map[string][]string{}
	if query.StartTime != "" {
		params["start_time"] = []string{query.StartTime}
	}
	if query.EndTime != "" {
		params["end_time"] = []string{query.EndTime}
	}

	resp, err := c.apiClient.Logical().ReadRawWithDataWithContext(ctx, query.Endpoint(), params)
	if err != nil {
		return nil, fmt.Errorf("get activity from %s: %w", query.Endpoint(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		if readErr != nil {
			return nil, fmt.Errorf("get activity from %s: unexpected status %d and failed to read body: %w", query.Endpoint(), resp.StatusCode, readErr)
		}

		return nil, fmt.Errorf("get activity from %s: unexpected status %d: %s", query.Endpoint(), resp.StatusCode, strings.TrimSpace(string(body)))
	}

	activity, err := decodeActivity(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decode activity response from %s: %w", query.Endpoint(), err)
	}

	return activity, nil
}

func decodeActivity(r io.Reader) (*MonthlyActivityData, error) {
	var decoded activityResponse
	if err := json.NewDecoder(r).Decode(&decoded); err != nil {
		return nil, err
	}

	return decoded.Data.normalize(), nil
}
