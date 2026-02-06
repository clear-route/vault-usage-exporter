# Basic Benchmark config options
vault_addr = "http://host.docker.internal:8200"
vault_token = "root"
vault_namespace="root"
duration = "30s"

# cleanup = true

test "approle_auth" "approle_logins" {
  weight = 10
  config {
    role {
      role_name = "benchmark-role"
      token_ttl="2m"
    }
  }
}

test "kvv2_read" "kvv2_read_test" {
    weight = 20
    config {
        numkvs = 100
    }
}

test "kvv2_write" "kvv2_write_test" {
    weight = 30
    config {
        numkvs = 10
        kvsize = 1000
    }
}

test "userpass_auth" "userpass_test1" {
    weight = 40
    config {
        username = "test-user"
        password = "password"
    }
}