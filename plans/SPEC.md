# Vault-Usage-Reporter

This spec outlines the implementation for a Prometheus Exporter for HashiCorp Vault Usage (Namespaces, Secret Engines, Auth Methods, Leases, Tokens. ...)

## General
- invoke the exporter in a `main.go` file
- create a channel that listens for cancellation through signals
- create a root context that is used throughout the lifetime of the exporter
- accept a path to a config yaml file that you will need to parse using https://github.com/go-yaml/yaml
- include the go process collector metrics
- use `vault_usage` as the metrics prefix for vault specific metrics

### Internal
- put internal packages in `internal/`
- parse the yaml file, validate it using https://github.com/go-playground/validator
- for now the config file looks like this; it will be extended later

```yaml
server:
  address: http://localhost:8200

auth_methods:
  enabled: true

secret_engines:
  enabled: true
```

### Vault
- put your vault client code in `pkg/vault/`
- use contexts for any Vault API call
- use  github.com/hashicorp/vault/api for Vault API calls

## Testing
- use stretchr/testify and the table driven test pattern for unit tests
- use testcontainers to spin up a vault container for integration tests

### API
- `/sys/namespaces`; List namespaces
- `/sys/auth`; List auth methods
- `/sys/mounts`; List mounted secret engines
- `/sys/leases/lookup/:prefix`; List leases

### Metrics
- `vault_usage_namespace{name=<namespace_name>}`; gauge
- `vault_usage_secret_engine{name=<name>,type=<type>,path=<path>,namespace=<namespace>}`; gauge
- `vault_usage_auth_method{name=<name>,type=<type>,path=<path>,namespace=<namespace>}`; gauge
- `vault_usage_leases{namespace=<namespace>}`; gauge
- `vault_usage_tokens{namespace=<namespace>}`; gauge
