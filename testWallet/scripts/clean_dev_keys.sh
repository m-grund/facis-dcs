#!/usr/bin/env bash
# Remove generated OID4VP demo material (safe to re-create with generate_dev_keys.py).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

delete_glob() {
  local pattern="$1"
  shopt -s nullglob
  local files=($pattern)
  shopt -u nullglob
  if ((${#files[@]} == 0)); then
    echo "skip (none): $pattern"
    return
  fi
  for f in "${files[@]}"; do
    rm -f "$f"
    echo "deleted: $f"
  done
}

delete_file() {
  local path="$1"
  if [[ -f "$path" ]]; then
    rm -f "$path"
    echo "deleted: $path"
  else
    echo "skip (missing): $path"
  fi
}

delete_glob "$ROOT/testWallet/keys/"*.jwk
delete_glob "$ROOT/testWallet/credentials/"*.json
delete_file "$ROOT/backend/config/oid4vp/trust.dev.json"

echo "done"
