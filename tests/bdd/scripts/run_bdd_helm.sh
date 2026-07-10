#!/usr/bin/env bash
set -euo pipefail

cleanup() {
  if [[ -f .tmp/port-forward-db.pid ]]; then
    kill "$(cat .tmp/port-forward-db.pid)" >/dev/null 2>&1 || true
  fi
  if [[ -f .tmp/port-forward-dcs.pid ]]; then
    kill "$(cat .tmp/port-forward-dcs.pid)" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

: "${VENV_PATH:?VENV_PATH is required}"
: "${FEATURES_PATH:?FEATURES_PATH is required}"
: "${KUBECTL_BIN:?KUBECTL_BIN is required}"
: "${K8S_NAMESPACE:?K8S_NAMESPACE is required}"
: "${DCS_DEPLOYMENT:?DCS_DEPLOYMENT is required}"
: "${BDD_DCS_BASE_URL:?BDD_DCS_BASE_URL is required}"
: "${PROJECT_ROOT:?PROJECT_ROOT is required}"

BDD_PUBLIC_ORIGIN="${BDD_PUBLIC_ORIGIN:-http://localhost:18080}"
export BDD_PUBLIC_ORIGIN
export STATUSLIST_SERVICE_URL="${STATUSLIST_SERVICE_URL:-${BDD_PUBLIC_ORIGIN}/statuslist}"

# Sign did:web challenges through the in-cluster token: the BDD harness has no
# local SoftHSM token in the Helm/kind harness (Workstream A: keys are
# non-extractable, PKCS#11-only). Resolve the pod by label rather than
# `exec deploy/...`: the DCS deployment's selector also matches pdf-core pods
# (no component label in matchLabels), so kubectl's deploy→pod resolution can
# pick a pod that has no digital-contracting-service container.
DCS_POD="$("${KUBECTL_BIN}" -n "${K8S_NAMESPACE}" get pod \
  -l app.kubernetes.io/component=backend \
  --field-selector=status.phase=Running \
  -o jsonpath='{.items[0].metadata.name}')"
export BDD_HSMSIGN_EXEC="${KUBECTL_BIN} -n ${K8S_NAMESPACE} exec ${DCS_POD} -c digital-contracting-service --"

mkdir -p .tmp .reports/junit
REPORTS_JUNIT_DIR="$PWD/.reports/junit"

DCS_HEALTH_URL="${BDD_DCS_BASE_URL%/}/auth/login"

verify_host_ingress() {
  local body http_code
  body=$(curl -s -X POST "$DCS_HEALTH_URL" -H 'Content-Type: application/json' -d '{}' 2>/dev/null || true)
  http_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$DCS_HEALTH_URL" \
    -H 'Content-Type: application/json' -d '{}' 2>/dev/null || echo "000")
  if [[ "$body" == "404 page not found" ]] || [[ "$http_code" == "404" && "$body" == *"page not found"* ]]; then
    echo "Host port 18080 is not reaching the kind Traefik ingress (got Go default 404)."
    echo "Ensure kind exposes port 18080 and the BDD stack is deployed: make -C tests/bdd kind_up"
    return 1
  fi
  return 0
}

wait_for_dcs_http() {
  local deadline=$(( $(date +%s) + 120 ))
  local http_code
  until http_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$DCS_HEALTH_URL" \
      -H 'Content-Type: application/json' -d '{}' 2>/dev/null) \
      && [[ "$http_code" =~ ^[24][0-9]{2}$ ]] && [[ "$http_code" != 404 ]]; do
    if [ "$(date +%s)" -gt "$deadline" ]; then
      echo "Timed out waiting for DCS HTTP on $DCS_HEALTH_URL"
      verify_host_ingress || true
      echo "Ensure kind exposes port 18080 and the BDD stack is deployed: make -C tests/bdd kind_up"
      "$KUBECTL_BIN" get pods -n kube-system -l app.kubernetes.io/name=traefik -o wide || true
      return 1
    fi
    sleep 2
  done
}

echo "Waiting for DCS deployment ($DCS_DEPLOYMENT) to be available"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=available --timeout=180s "deployment/$DCS_DEPLOYMENT"
echo "Waiting for DCS backend pod to accept traffic"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=ready pod \
  -l "app.kubernetes.io/name=digital-contracting-service,app.kubernetes.io/component=backend" \
  --timeout=180s

echo "Waiting for DCS HTTP via Traefik ingress at $DCS_HEALTH_URL ..."
if ! verify_host_ingress; then
  exit 1
fi
wait_for_dcs_http
echo "DCS is reachable at $DCS_HEALTH_URL"

echo "Starting port-forward for PostgreSQL"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" port-forward "svc/dcs-postgresql" 5432:5432 > .tmp/port-forward-db.log 2>&1 &
echo $! > .tmp/port-forward-db.pid

deadline=$(( $(date +%s) + 30 ))
until nc -z 127.0.0.1 5432 2>/dev/null; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for port-forward on 5432"
    cat .tmp/port-forward-db.log || true
    exit 1
  fi
  sleep 1
done
echo "Port-forward on 5432 is ready"

# Direct service access for endpoints Traefik does not route (e.g. /metrics,
# which the backend serves at its root, outside the API prefix).
DCS_SERVICE="${DCS_SERVICE:-$DCS_DEPLOYMENT}"
LOCAL_FORWARD_PORT="${LOCAL_FORWARD_PORT:-18991}"
SERVICE_PORT="${SERVICE_PORT:-8991}"
echo "Starting port-forward for DCS service ($DCS_SERVICE)"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" port-forward "svc/$DCS_SERVICE" \
  "$LOCAL_FORWARD_PORT:$SERVICE_PORT" > .tmp/port-forward-dcs.log 2>&1 &
echo $! > .tmp/port-forward-dcs.pid

deadline=$(( $(date +%s) + 30 ))
until nc -z 127.0.0.1 "$LOCAL_FORWARD_PORT" 2>/dev/null; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for port-forward on $LOCAL_FORWARD_PORT"
    cat .tmp/port-forward-dcs.log || true
    exit 1
  fi
  sleep 1
done
export BDD_DCS_INTERNAL_ORIGIN="http://localhost:$LOCAL_FORWARD_PORT"
echo "Port-forward on $LOCAL_FORWARD_PORT is ready"

source "$VENV_PATH/bin/activate"
export BDD_DCS_BASE_URL

echo "Checking statuslist for BDD at $STATUSLIST_SERVICE_URL"
python "$PWD/scripts/ensure_statuslist_for_bdd.py"

export DATABASE_URL="host=localhost port=5432 user=dcs password=dcs dbname=dcs sslmode=disable"

# Canonical bdd-executor integration requires the package in the active environment.
python -c 'import eu.xfsc.bdd.core' >/dev/null

EXTRA_ARGS=()
if [[ -n "${ARG_BDD:-}" ]]; then
  # shellcheck disable=SC2206
  EXTRA_ARGS=(${ARG_BDD})
fi

JUNIT_ARGS=(--junit --junit-directory .reports/junit)
if [[ -n "${ARG_BDD_JUNIT:-}" ]]; then
  # shellcheck disable=SC2206
  JUNIT_ARGS=(${ARG_BDD_JUNIT})
fi

echo "Running BDD suite via bdd-executor environment"
cd "$PROJECT_ROOT"
"$VENV_PATH/bin/coverage" run --append -m behave "${JUNIT_ARGS[@]}" "$FEATURES_PATH" "${EXTRA_ARGS[@]}"

JUNIT_COUNT=$(find "$REPORTS_JUNIT_DIR" -name "*.xml" 2>/dev/null | wc -l || true)
echo "Generated $JUNIT_COUNT junit XML files in $REPORTS_JUNIT_DIR/"
