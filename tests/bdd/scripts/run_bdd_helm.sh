#!/usr/bin/env bash
set -euo pipefail

cleanup() {
  if [[ -f .tmp/port-forward-db.pid ]]; then
    kill "$(cat .tmp/port-forward-db.pid)" >/dev/null 2>&1 || true
  fi
  if [[ -f .tmp/port-forward-dcs.pid ]]; then
    kill "$(cat .tmp/port-forward-dcs.pid)" >/dev/null 2>&1 || true
  fi
  if [[ -f .tmp/port-forward-orce.pid ]]; then
    kill "$(cat .tmp/port-forward-orce.pid)" >/dev/null 2>&1 || true
  fi
  if [[ -f .tmp/port-forward-dss.pid ]]; then
    kill "$(cat .tmp/port-forward-dss.pid)" >/dev/null 2>&1 || true
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
# Scopes every pod/label lookup below to THIS release: the two-instance BDD
# suite deploys a second DCS release (dcs2) into the SAME namespace
# (tests/bdd/Makefile's kind_deploy_b), and app.kubernetes.io/component=backend
# alone matches both releases' backend pods — an unscoped selector previously
# caused a wrong-pod log dump / signing-exec pick here.
: "${HELM_RELEASE:?HELM_RELEASE is required}"

BDD_PUBLIC_ORIGIN="${BDD_PUBLIC_ORIGIN:-http://localhost:18080}"
export BDD_PUBLIC_ORIGIN
export STATUSLIST_SERVICE_URL="${STATUSLIST_SERVICE_URL:-${BDD_PUBLIC_ORIGIN}/statuslist}"

# BDD_DCS_BASE_URL_A / _B: the two-instance (@two-instance) peer-trust
# scenarios (steps/peer_trust/dcs_peer_trust_steps.py) address instance A and
# instance B independently of the single-instance BDD_DCS_BASE_URL used by
# every other scenario. Instance A is conventionally "the" default instance
# in this Helm/kind harness, so _A is just an alias for BDD_DCS_BASE_URL;
# _B defaults to the dcs2 release's public origin (values.bdd2.yml).
export BDD_DCS_BASE_URL_A="$BDD_DCS_BASE_URL"
export BDD_DCS_BASE_URL_B="${BDD_DCS_BASE_URL_B:-http://dcs-b.localhost:18080/digital-contracting-service/api}"

# Sign did:web challenges through the in-cluster token: the BDD harness has no
# local SoftHSM token in the Helm/kind harness (keys are
# non-extractable, PKCS#11-only). Resolve the pod by label rather than
# `exec deploy/...`: the DCS deployment's selector also matches pdf-core pods
# (no component label in matchLabels), so kubectl's deploy→pod resolution can
# pick a pod that has no digital-contracting-service container. Scoped by
# instance (see HELM_RELEASE above) so this always signs through instance A's
# own token, never instance B's, when both releases share the namespace.
DCS_POD="$("${KUBECTL_BIN}" -n "${K8S_NAMESPACE}" get pod \
  -l "app.kubernetes.io/component=backend,app.kubernetes.io/instance=${HELM_RELEASE}" \
  --field-selector=status.phase=Running \
  -o jsonpath='{.items[0].metadata.name}')"
export BDD_HSMSIGN_EXEC="${KUBECTL_BIN} -n ${K8S_NAMESPACE} exec ${DCS_POD} -c digital-contracting-service --"

# IPFS CID-swap tamper seam (steps/support/tamper_seam.py): several
# verify-shaped endpoints always re-fetch the SERVER'S OWN stored PDF from
# IPFS by CID, so tampered-artifact scenarios inject bytes as a NEW CID via
# `ipfs add` exec'd inside the shared IPFS pod, then repoint the owning row's
# CID column at it (via the existing context.db test-DB connection). IPFS is
# a SINGLE instance shared across both BDD releases (values.bdd2.yml's
# ipfsClient.mfsBaseURL points at "dcs-ipfs" regardless of caller instance),
# so this is not release-scoped the way BDD_HSMSIGN_EXEC is.
IPFS_POD="$("${KUBECTL_BIN}" -n "${K8S_NAMESPACE}" get pod \
  -l "app.kubernetes.io/name=ipfs,app.kubernetes.io/instance=dcs" \
  --field-selector=status.phase=Running \
  -o jsonpath='{.items[0].metadata.name}')"
# -i/--stdin is required (not just harmless) here: `ipfs add -` reads its
# content from stdin, and without --stdin the API server may not have a
# stdin stream attached before the remote command starts reading — observed
# in practice as an intermittent race where `ipfs add` silently succeeds
# against an EMPTY stdin (producing the well-known empty-file CID
# Qmb...4Q7Vs-style hash) instead of the intended bytes, rather than a
# reliable failure.
export BDD_IPFS_EXEC="${KUBECTL_BIN} -n ${K8S_NAMESPACE} exec -i ${IPFS_POD} --"

mkdir -p .tmp .reports/junit
REPORTS_JUNIT_DIR="$PWD/.reports/junit"

# Emits `--resolve <host>:<port>:127.0.0.1` for a URL's host[:port], so
# *.localhost hostnames the host machine's own resolver may not know (e.g.
# dcs-b.localhost, which nothing registers anywhere) resolve to loopback for
# curl's own DNS resolution without /etc/hosts or sudo (USER CONSTRAINT: no
# /etc/hosts writes, no sudo anywhere in this harness). This is independent
# of environment.py's socket.getaddrinfo fallback, which only covers the
# Python behave process, not shell-level curl calls like the ones below.
resolve_args_for_url() {
  local url="$1" hostport host port
  hostport="${url#*://}"
  hostport="${hostport%%/*}"
  host="${hostport%%:*}"
  if [[ "$hostport" == *:* ]]; then
    port="${hostport##*:}"
  else
    case "$url" in
      https://*) port=443 ;;
      *) port=80 ;;
    esac
  fi
  printf '%s\n' "--resolve" "${host}:${port}:127.0.0.1"
}

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
  # Generous: on a cold cluster the backend blocks its HTTP server on the
  # Federated Catalogue schema sync, and FC's own first boot takes minutes.
  local deadline=$(( $(date +%s) + 900 ))
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
  -l "app.kubernetes.io/name=digital-contracting-service,app.kubernetes.io/component=backend,app.kubernetes.io/instance=${HELM_RELEASE}" \
  --timeout=180s

echo "Waiting for DCS HTTP via Traefik ingress at $DCS_HEALTH_URL ..."
if ! verify_host_ingress; then
  exit 1
fi
wait_for_dcs_http
echo "DCS is reachable at $DCS_HEALTH_URL"

# The Federated Catalogue's /verification endpoint needs Neo4j and its
# schema cache warm before it answers within the DCS client timeout;
# template registration flows (features/02, template_archive) fail with
# gateway timeouts when the suite starts against a cold FC. Warm it with
# real verification requests through a temporary port-forward until one
# completes, however it completes.
wait_for_fc_verification() {
  local fc_deploy="${HELM_RELEASE}-federated-catalogue"
  if ! "$KUBECTL_BIN" -n "$K8S_NAMESPACE" get "deployment/$fc_deploy" >/dev/null 2>&1; then
    return 0
  fi
  echo "Warming the Federated Catalogue verification endpoint ($fc_deploy)"
  "$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=available --timeout=300s "deployment/$fc_deploy"
  "$KUBECTL_BIN" -n "$K8S_NAMESPACE" port-forward "deployment/$fc_deploy" 18581:8081 >/dev/null 2>&1 &
  local pf_pid=$!
  local deadline=$(( $(date +%s) + 300 ))
  local warmed=1
  sleep 2
  until curl -s -o /dev/null --max-time 8 -X POST \
      "http://localhost:18581/verification?verifySchema=true&verifySemantics=true&verifySignatures=false&verifyVCSignature=false&verifyVPSignature=false" \
      -H 'Content-Type: application/json' -d '{}' 2>/dev/null; do
    if [ "$(date +%s)" -gt "$deadline" ]; then
      echo "Timed out warming the Federated Catalogue verification endpoint"
      warmed=0
      break
    fi
    sleep 5
  done
  kill "$pf_pid" >/dev/null 2>&1 || true
  if [ "$warmed" -eq 1 ]; then
    echo "Federated Catalogue verification endpoint is responding"
  else
    return 1
  fi
}
wait_for_fc_verification

# Instance B (dcs2, features/17_peer_trust @two-instance): only checked when
# the caller tells us it exists (DCS_DEPLOYMENT_B set AND actually present in
# this namespace) — never silently skipped without saying so, since a
# missing/unready instance B means every @two-instance scenario will fail
# with a much less obvious error later.
if [[ -n "${DCS_DEPLOYMENT_B:-}" ]] && "$KUBECTL_BIN" -n "$K8S_NAMESPACE" get "deployment/$DCS_DEPLOYMENT_B" >/dev/null 2>&1; then
  echo "Waiting for DCS deployment B ($DCS_DEPLOYMENT_B) to be available"
  "$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=available --timeout=180s "deployment/$DCS_DEPLOYMENT_B"

  BDD_PUBLIC_ORIGIN_B="${BDD_PUBLIC_ORIGIN_B:-http://dcs-b.localhost:18080}"
  DCS_HEALTH_URL_B="${BDD_DCS_BASE_URL_B%/}/auth/login"
  mapfile -t CURL_RESOLVE_B < <(resolve_args_for_url "$BDD_PUBLIC_ORIGIN_B")

  echo "Waiting for DCS HTTP via Traefik ingress (instance B) at $DCS_HEALTH_URL_B ..."
  deadline_b=$(( $(date +%s) + 120 ))
  http_code_b=""
  until http_code_b=$(curl -s "${CURL_RESOLVE_B[@]}" -o /dev/null -w "%{http_code}" -X POST "$DCS_HEALTH_URL_B" \
      -H 'Content-Type: application/json' -d '{}' 2>/dev/null) \
      && [[ "$http_code_b" =~ ^[24][0-9]{2}$ ]] && [[ "$http_code_b" != 404 ]]; do
    if [ "$(date +%s)" -gt "$deadline_b" ]; then
      echo "WARNING: timed out waiting for instance B's DCS HTTP on $DCS_HEALTH_URL_B — @two-instance scenarios will fail." >&2
      break
    fi
    sleep 2
  done
  if [[ "$http_code_b" =~ ^[24][0-9]{2}$ ]] && [[ "$http_code_b" != 404 ]]; then
    echo "DCS instance B is reachable at $DCS_HEALTH_URL_B"
  fi
else
  echo "WARNING: DCS_DEPLOYMENT_B is not set or not present in namespace $K8S_NAMESPACE — instance B" >&2
  echo "readiness was NOT verified. @two-instance BDD scenarios will fail if they run. Deploy it with" >&2
  echo "'make -C tests/bdd kind_deploy_b' (or kind_up, which now includes it) if you need instance B." >&2
fi

# The harness owns these loopback ports. A survivor forward from an earlier
# run — possibly against a DIFFERENT cluster/kubeconfig — binds first, the
# nc readiness check below then passes against the squatter, and every
# DB/ORCE test seam silently talks to the wrong stack.
for harness_port in 5432 18991 18880; do
  fuser -k -n tcp "$harness_port" >/dev/null 2>&1 || true
done
sleep 1

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

# The wallet-driven signing scenarios call the EU DSS demonstration webapp
# (charts/dss) as the external SCA that computes getDataToSign/signDocument.
# It is an in-cluster ClusterIP service; the harness reaches it through a
# port-forward at the localhost:18099 default that BDD_DSS_URL points at. The
# DSS Tomcat bundle boots slowly (readiness initialDelaySeconds 90), so allow a
# generous availability timeout before forwarding.
DSS_DEPLOYMENT="${HELM_RELEASE}-dss"
DSS_SERVICE="${HELM_RELEASE}-dss"
DSS_LOCAL_FORWARD_PORT="${DSS_LOCAL_FORWARD_PORT:-18099}"
echo "Waiting for DSS deployment ($DSS_DEPLOYMENT) to be available"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=available --timeout=420s "deployment/$DSS_DEPLOYMENT"

echo "Starting port-forward for DSS service ($DSS_SERVICE)"
KUBECTL_BIN="$KUBECTL_BIN" K8S_NAMESPACE="$K8S_NAMESPACE" \
  SERVICE_NAME="$DSS_SERVICE" PORT_MAPPING="$DSS_LOCAL_FORWARD_PORT:8080" \
  bash "$PWD/scripts/keep_port_forward.sh" > .tmp/port-forward-dss.log 2>&1 &
echo $! > .tmp/port-forward-dss.pid

deadline=$(( $(date +%s) + 30 ))
until nc -z 127.0.0.1 "$DSS_LOCAL_FORWARD_PORT" 2>/dev/null; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for DSS port-forward on $DSS_LOCAL_FORWARD_PORT"
    cat .tmp/port-forward-dss.log || true
    exit 1
  fi
  sleep 1
done
export BDD_DSS_URL="http://localhost:$DSS_LOCAL_FORWARD_PORT"
echo "DSS port-forward on $DSS_LOCAL_FORWARD_PORT is ready"

# Archive notary and audit-log endpoints are intentionally not exposed by the
# public ORCE ingress. Reach the release-scoped service directly and obtain the
# configured token from the running pod rather than duplicating it here.
ORCE_DEPLOYMENT="${HELM_RELEASE}-orce"
ORCE_SERVICE="${HELM_RELEASE}-orce"
ORCE_LOCAL_FORWARD_PORT="${ORCE_LOCAL_FORWARD_PORT:-18880}"
echo "Waiting for ORCE deployment ($ORCE_DEPLOYMENT) to be available"
"$KUBECTL_BIN" -n "$K8S_NAMESPACE" wait --for=condition=available --timeout=180s "deployment/$ORCE_DEPLOYMENT"
# During a rollout the terminating pod still reports phase Running while its
# containers are already gone — pick the newest running pod and retry the
# exec until it answers.
ORCE_TOKEN=""
deadline=$(( $(date +%s) + 120 ))
while [[ -z "$ORCE_TOKEN" ]]; do
  ORCE_POD="$("$KUBECTL_BIN" -n "$K8S_NAMESPACE" get pod \
    -l "app.kubernetes.io/name=orce,app.kubernetes.io/instance=${HELM_RELEASE}" \
    --field-selector=status.phase=Running \
    --sort-by=.metadata.creationTimestamp \
    -o jsonpath='{.items[-1:].metadata.name}' 2>/dev/null || true)"
  if [[ -n "$ORCE_POD" ]]; then
    ORCE_TOKEN="$("$KUBECTL_BIN" -n "$K8S_NAMESPACE" exec "$ORCE_POD" -c orce -- \
      printenv ORCE_ARCHIVE_AUDIT_LOG_BEARER_TOKEN 2>/dev/null || true)"
  fi
  if [[ -z "$ORCE_TOKEN" ]]; then
    if [ "$(date +%s)" -gt "$deadline" ]; then
      echo "ORCE archive audit token is not configured in pod ${ORCE_POD:-<none>}" >&2
      exit 1
    fi
    sleep 3
  fi
done

echo "Starting port-forward for ORCE service ($ORCE_SERVICE)"
KUBECTL_BIN="$KUBECTL_BIN" K8S_NAMESPACE="$K8S_NAMESPACE" \
  SERVICE_NAME="$ORCE_SERVICE" PORT_MAPPING="$ORCE_LOCAL_FORWARD_PORT:1880" \
  bash "$PWD/scripts/keep_port_forward.sh" > .tmp/port-forward-orce.log 2>&1 &
echo $! > .tmp/port-forward-orce.pid

deadline=$(( $(date +%s) + 30 ))
until nc -z 127.0.0.1 "$ORCE_LOCAL_FORWARD_PORT" 2>/dev/null; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for ORCE port-forward on $ORCE_LOCAL_FORWARD_PORT"
    cat .tmp/port-forward-orce.log || true
    exit 1
  fi
  sleep 1
done
export BDD_ORCE_ARCHIVE_NOTARY_URL="http://localhost:${ORCE_LOCAL_FORWARD_PORT}/archive/notary"
export BDD_ORCE_ARCHIVE_AUDIT_LOG_URL="http://localhost:${ORCE_LOCAL_FORWARD_PORT}/archive-audit-events.jsonl"
export BDD_ORCE_ARCHIVE_AUDIT_LOG_BEARER_TOKEN="$ORCE_TOKEN"
export BDD_ORCE_NAMESPACE="$K8S_NAMESPACE"
export BDD_ORCE_DEPLOYMENT="$ORCE_DEPLOYMENT"
export BDD_KUBECTL="$KUBECTL_BIN"

echo "Waiting for authenticated ORCE archive audit-log endpoint"
deadline=$(( $(date +%s) + 60 ))
orce_archive_code=""
until orce_archive_code=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer $ORCE_TOKEN" "$BDD_ORCE_ARCHIVE_AUDIT_LOG_URL" 2>/dev/null) \
    && [[ "$orce_archive_code" == "200" || "$orce_archive_code" == "404" ]]; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for ORCE archive audit log (last HTTP $orce_archive_code)"
    exit 1
  fi
  sleep 2
done
echo "ORCE archive endpoints are reachable"

# ORCE (Node-RED) hosts the contract-target-flow the deployment scenarios POST
# to directly; the BDD values route it through the shared Traefik ingress
# (orce.ingress in values.bdd.yml), so it is reachable at the public origin —
# same path locally and on CI, no port-forward. An empty POST must yield the
# flow's own 400 validation error; a Traefik 404 means the route is missing.
export BDD_ORCE_TARGET_URL="${BDD_ORCE_TARGET_URL:-${BDD_PUBLIC_ORIGIN}/contract-target/deploy}"
echo "Waiting for ORCE contract-target flow at $BDD_ORCE_TARGET_URL ..."
deadline=$(( $(date +%s) + 60 ))
until orce_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BDD_ORCE_TARGET_URL" \
    -H 'Content-Type: application/json' -d '{}' 2>/dev/null) \
    && [[ "$orce_code" =~ ^[24][0-9]{2}$|^400$ ]] && [[ "$orce_code" != 404 ]]; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "Timed out waiting for ORCE contract-target flow at $BDD_ORCE_TARGET_URL (last HTTP $orce_code)"
    exit 1
  fi
  sleep 2
done
echo "ORCE contract-target flow is reachable (HTTP $orce_code); BDD_ORCE_TARGET_URL=$BDD_ORCE_TARGET_URL"

source "$VENV_PATH/bin/activate"
export BDD_DCS_BASE_URL

echo "Checking statuslist for BDD at $STATUSLIST_SERVICE_URL"
python "$PWD/scripts/ensure_statuslist_for_bdd.py"

export DATABASE_URL="host=localhost port=5432 user=dcs password=dcs dbname=dcs sslmode=disable"

# Canonical bdd-executor integration requires the package in the active environment.
python -c 'import eu.xfsc.bdd.core' >/dev/null

# Isolated-stack features (clean-DB assumptions, component restarts) run in
# their dedicated targets, not the shared full-suite stack. Callers that DO
# provide the isolation (run_bdd_audit_kind_once) override ARG_BDD_TAGS.
EXTRA_ARGS=(${ARG_BDD_TAGS---tags=-isolated_stack})
if [[ -n "${ARG_BDD:-}" ]]; then
  # shellcheck disable=SC2206
  EXTRA_ARGS+=(${ARG_BDD})
fi

JUNIT_ARGS=(--junit --junit-directory .reports/junit)
if [[ -n "${ARG_BDD_JUNIT:-}" ]]; then
  # shellcheck disable=SC2206
  JUNIT_ARGS=(${ARG_BDD_JUNIT})
fi

# The deployed stack + all its port-forwards (DSS 18099, ORCE, DB, instance B)
# are live at this point and stay alive until this script exits (trap cleanup).
# RUN_MODE selects what runs against them without tearing anything down:
#   bdd (default) — the behave suite via the bdd-executor environment;
#   e2e           — the Playwright suite (its own vite servers + the venv-backed
#                   signing helpers), so the frontend E2E gets the same live
#                   two-instance stack + DSS forward the BDD suite uses.
if [[ "${RUN_MODE:-bdd}" == "e2e" ]]; then
  echo "Running Playwright E2E against the deployed stack"
  cd "$PROJECT_ROOT/frontend/ClientApp"
  E2E_DCS_API_BASE="${BDD_PUBLIC_ORIGIN}/digital-contracting-service/api" \
  E2E_BDD_PYTHON="$VENV_PATH/bin/python3" \
    npm run e2e
else
  echo "Running BDD suite via bdd-executor environment"
  cd "$PROJECT_ROOT"
  "$VENV_PATH/bin/coverage" run --append -m behave "${JUNIT_ARGS[@]}" "$FEATURES_PATH" "${EXTRA_ARGS[@]}"

  JUNIT_COUNT=$(find "$REPORTS_JUNIT_DIR" -name "*.xml" 2>/dev/null | wc -l || true)
  echo "Generated $JUNIT_COUNT junit XML files in $REPORTS_JUNIT_DIR/"
fi
