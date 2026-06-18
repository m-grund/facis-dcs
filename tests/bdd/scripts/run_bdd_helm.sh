#!/usr/bin/env bash
set -euo pipefail

cleanup() {
  if [[ -f .tmp/port-forward.pid ]]; then
    kill "$(cat .tmp/port-forward.pid)" >/dev/null 2>&1 || true
  fi
  if [[ -f .tmp/port-forward-hydra.pid ]]; then
    kill "$(cat .tmp/port-forward-hydra.pid)" >/dev/null 2>&1 || true
  fi
  if [[ -f .tmp/port-forward-db.pid ]]; then
    kill "$(cat .tmp/port-forward-db.pid)" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

: "${VENV_PATH:?VENV_PATH is required}"
: "${FEATURES_PATH:?FEATURES_PATH is required}"
: "${KUBECTL_BIN:?KUBECTL_BIN is required}"
: "${K8S_NAMESPACE:?K8S_NAMESPACE is required}"
: "${DCS_DEPLOYMENT:?DCS_DEPLOYMENT is required}"
: "${DCS_SERVICE:?DCS_SERVICE is required}"
: "${HYDRA_SERVICE:?HYDRA_SERVICE is required}"
: "${LOCAL_HYDRA_PORT:?LOCAL_HYDRA_PORT is required}"
: "${HYDRA_SERVICE_PORT:?HYDRA_SERVICE_PORT is required}"
: "${BDD_HYDRA_PUBLIC_ISSUER_URL:?BDD_HYDRA_PUBLIC_ISSUER_URL is required}"
: "${LOCAL_FORWARD_PORT:?LOCAL_FORWARD_PORT is required}"
: "${SERVICE_PORT:?SERVICE_PORT is required}"
: "${DCS_API_BASE_PATH:?DCS_API_BASE_PATH is required}"
: "${PROJECT_ROOT:?PROJECT_ROOT is required}"

mkdir -p .tmp .reports/junit
REPORTS_JUNIT_DIR="$PWD/.reports/junit"

DCS_HEALTH_URL="http://127.0.0.1:${LOCAL_FORWARD_PORT}${DCS_API_BASE_PATH}/auth/login"

wait_for_dcs_http() {
  local deadline=$(( $(date +%s) + 60 ))
  local http_code
  until http_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$DCS_HEALTH_URL" \
      -H 'Content-Type: application/json' -d '{}' 2>/dev/null) \
      && [[ "$http_code" =~ ^[24][0-9]{2}$ ]] && [[ "$http_code" != 404 ]]; do
    if [ "$(date +%s)" -gt "$deadline" ]; then
      echo "Timed out waiting for DCS HTTP on $DCS_HEALTH_URL"
      cat .tmp/port-forward.log || true
      return 1
    fi
    if [[ -f .tmp/port-forward.pid ]] && ! kill -0 "$(cat .tmp/port-forward.pid)" 2>/dev/null; then
      echo "DCS port-forward exited unexpectedly"
      cat .tmp/port-forward.log || true
      return 1
    fi
    sleep 1
  done
}

start_dcs_port_forward() {
  if [[ -f .tmp/port-forward.pid ]]; then
    kill "$(cat .tmp/port-forward.pid)" >/dev/null 2>&1 || true
  fi
  echo "Starting port-forward svc/$DCS_SERVICE $LOCAL_FORWARD_PORT:$SERVICE_PORT in namespace $K8S_NAMESPACE"
  "$KUBECTL_BIN" -n "$K8S_NAMESPACE" port-forward "svc/$DCS_SERVICE" "$LOCAL_FORWARD_PORT:$SERVICE_PORT" > .tmp/port-forward.log 2>&1 &
  echo $! > .tmp/port-forward.pid
}

echo "Waiting for DCS deployment ($DCS_DEPLOYMENT) to be available"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=available --timeout=180s "deployment/$DCS_DEPLOYMENT"
echo "Waiting for DCS pod to accept traffic"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=ready pod \
  -l "app.kubernetes.io/name=digital-contracting-service" \
  --timeout=180s

start_dcs_port_forward
echo "Waiting for DCS HTTP via port-forward on $LOCAL_FORWARD_PORT..."
if ! wait_for_dcs_http; then
  echo "Retrying DCS port-forward once"
  start_dcs_port_forward
  wait_for_dcs_http
fi
echo "DCS is reachable at $DCS_HEALTH_URL"

echo "Starting port-forward svc/$HYDRA_SERVICE $LOCAL_HYDRA_PORT:$HYDRA_SERVICE_PORT in namespace $K8S_NAMESPACE"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" port-forward "svc/$HYDRA_SERVICE" "$LOCAL_HYDRA_PORT:$HYDRA_SERVICE_PORT" > .tmp/port-forward-hydra.log 2>&1 &
echo $! > .tmp/port-forward-hydra.pid

deadline=$(( $(date +%s) + 30 ))
until nc -z 127.0.0.1 "$LOCAL_HYDRA_PORT" 2>/dev/null; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for hydra port-forward on $LOCAL_HYDRA_PORT"
    cat .tmp/port-forward-hydra.log || true
    exit 1
  fi
  sleep 1
done
echo "Hydra port-forward on $LOCAL_HYDRA_PORT is ready"
echo "Hydra public issuer URL configured as: $BDD_HYDRA_PUBLIC_ISSUER_URL"

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

source "$VENV_PATH/bin/activate"
export BDD_DCS_BASE_URL="http://127.0.0.1:$LOCAL_FORWARD_PORT$DCS_API_BASE_PATH"

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
