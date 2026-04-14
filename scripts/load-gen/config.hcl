# Basic benchmark config options.
vault_addr = "http://host.docker.internal:8200"
vault_token = "root"
vault_namespace = "root"

# Keep the benchmark-created auth mounts around so the exporter can continue to see them.
cleanup = false

# Keep mounts unique across repeated runs since cleanup is disabled.
random_mounts = true

duration = "45s"
workers = 20
rps = 0
log_level = "INFO"

# This profile is auth-heavy on purpose:
# - AppRole, Userpass, and JWT all work against the local dev Vault setup.
# - Cert auth is intentionally not used here because the demo Vault listens on
#   plain HTTP, so client-certificate auth cannot succeed.
# - KV read/write stays in the mix so issued tokens are exercised after login.

test "approle_auth" "approle_auth" {
  weight = 22

  config {
    role {
      role_name = "benchmark-approle"
      bind_secret_id = true
      token_ttl = "15m"
      token_max_ttl = "30m"
      token_type = "service"
    }

    secret_id {
      ttl = "15m"
    }
  }
}

test "userpass_auth" "userpass_auth" {
  weight = 22

  config {
    username = "benchmark-user"
    password = "benchmark-password"
    token_ttl = "15m"
    token_max_ttl = "30m"
    token_type = "service"
  }
}

test "jwt_auth" "jwt_auth" {
  weight = 18

  config {
    # vault-benchmark can generate an internal signing key for this test, so no
    # external JWKS/OIDC provider is required for local runs.
    auth {
      bound_issuer = "vault-benchmark"
    }

    role {
      name = "benchmark-jwt-role"
      role_type = "jwt"
      bound_audiences = ["https://vault.plugin.auth.jwt.test"]
      user_claim = "https://vault/user"
      token_ttl = "15m"
      token_max_ttl = "30m"
      token_type = "service"
    }
  }
}

test "approle_auth" "approle_batch_auth" {
  weight = 18

  config {
    role {
      role_name = "benchmark-batch-approle"
      bind_secret_id = true
      token_ttl = "10m"
      token_max_ttl = "20m"
      token_type = "batch"
    }

    secret_id {
      ttl = "10m"
    }
  }
}

test "kvv2_read" "kvv2_read" {
  weight = 10

  config {
    numkvs = 200
  }
}

test "kvv2_write" "kvv2_write" {
  weight = 10

  config {
    numkvs = 50
    kvsize = 512
  }
}
