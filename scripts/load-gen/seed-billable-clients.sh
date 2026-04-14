#!/usr/bin/env bash

set -euo pipefail

VAULT_ADDR="${VAULT_ADDR:-http://127.0.0.1:8200}"
VAULT_TOKEN="${VAULT_TOKEN:-root}"
CLIENT_SEED_COUNT="${CLIENT_SEED_COUNT:-5}"
RUN_ID="${RUN_ID:-$(date +%s)}"

USERPASS_MOUNT="userpass-billable"
APPROLE_MOUNT="approle-billable"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

api() {
  local method="$1"
  local path="$2"
  local data="${3:-}"
  local token="${4:-$VAULT_TOKEN}"

  if [[ -n "$data" ]]; then
    curl -sf \
      --request "$method" \
      --header "X-Vault-Token: $token" \
      --header "Content-Type: application/json" \
      --data "$data" \
      "$VAULT_ADDR/v1/$path"
    return
  fi

  curl -sf \
    --request "$method" \
    --header "X-Vault-Token: $token" \
    "$VAULT_ADDR/v1/$path"
}

ensure_auth_mount() {
  local mount="$1"
  local auth_type="$2"

  if api GET "sys/auth" | jq -e --arg mount "${mount}/" 'has($mount)' >/dev/null; then
    return
  fi

  api POST "sys/auth/$mount" "{\"type\":\"$auth_type\"}" >/dev/null
}

seed_userpass_clients() {
  local i username password token

  ensure_auth_mount "$USERPASS_MOUNT" "userpass"

  for i in $(seq 1 "$CLIENT_SEED_COUNT"); do
    username="billable-user-${RUN_ID}-${i}"
    password="billable-password-${RUN_ID}-${i}"

    api POST "auth/$USERPASS_MOUNT/users/$username" "{\"password\":\"$password\"}" >/dev/null
    token="$(
      api POST "auth/$USERPASS_MOUNT/login/$username" "{\"password\":\"$password\"}" |
        jq -r '.auth.client_token'
    )"
    api GET "auth/token/lookup-self" "" "$token" >/dev/null
  done
}

seed_approle_clients() {
  local i role role_id secret_id token

  ensure_auth_mount "$APPROLE_MOUNT" "approle"

  for i in $(seq 1 "$CLIENT_SEED_COUNT"); do
    role="billable-role-${RUN_ID}-${i}"

    api POST "auth/$APPROLE_MOUNT/role/$role" '{"token_ttl":"15m","token_max_ttl":"30m","token_type":"service"}' >/dev/null
    role_id="$(
      api GET "auth/$APPROLE_MOUNT/role/$role/role-id" |
        jq -r '.data.role_id'
    )"
    secret_id="$(
      api POST "auth/$APPROLE_MOUNT/role/$role/secret-id" '{}' |
        jq -r '.data.secret_id'
    )"
    token="$(
      api POST "auth/$APPROLE_MOUNT/login" "{\"role_id\":\"$role_id\",\"secret_id\":\"$secret_id\"}" |
        jq -r '.auth.client_token'
    )"
    api GET "auth/token/lookup-self" "" "$token" >/dev/null
  done
}

seed_non_entity_clients() {
  local i policy_name policy token policy_hcl

  for i in $(seq 1 "$CLIENT_SEED_COUNT"); do
    policy_name="billable-nonentity-${RUN_ID}-${i}"
    policy_hcl='path "auth/token/lookup-self" { capabilities = ["read"] }'
    policy="$(jq -n --arg policy "$policy_hcl" '{policy:$policy}')"

    api PUT "sys/policies/acl/$policy_name" "$policy" >/dev/null
    token="$(
      api POST "auth/token/create-orphan" "$(jq -n --arg policy "$policy_name" '{policies:["default",$policy]}')" |
        jq -r '.auth.client_token'
    )"
    api GET "auth/token/lookup-self" "" "$token" >/dev/null
  done
}

print_summary() {
  api GET "sys/internal/counters/activity/monthly" |
    jq '{clients:.data.clients, entity_clients:.data.entity_clients, non_entity_clients:.data.non_entity_clients, by_namespace:.data.by_namespace}'
}

main() {
  require_cmd curl
  require_cmd jq

  seed_userpass_clients
  seed_approle_clients
  seed_non_entity_clients
  print_summary
}

main "$@"
