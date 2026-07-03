#!/bin/bash
# Full dev stack orchestration: Helm → pdf-core → Vite → Air
# Run from project root: bash dev-stack.sh
# Press Ctrl+C to stop everything
#
# Expected flow:
#   1. helm install dcs deployment/helm -f deployment/helm/values.dev.yml
#   2. bash dev-stack.sh   ← fetches certs, then starts pdf-core + frontend + backend

set -euo pipefail

trap 'kill $(jobs -p) 2>/dev/null; exit' INT TERM

# Config
HELM_RELEASE="dcs"
HELM_CHART_PATH="deployment/helm"
HELM_VALUES_FILE="deployment/helm/values.dev.yml"
K8S_NAMESPACE="default"
K8S_SECRET_NAME="dcs-crypto-provider-dev-cert-chain"
K8S_SECRET_KEY="chain.pem"
CERT_FILE="backend/certs/dev/chain.pem"
PDF_CORE_DIR="pdf-core"
PDF_CORE_DEV_ENV="$PDF_CORE_DIR/.dev.env"
PDF_CORE_ENV="$PDF_CORE_DIR/.env"

echo "=== Setting up dev environment ==="

# Helm: idempotent install-or-upgrade so this script works whether or not
# the release was already installed manually first.
echo "Updating Helm dependencies and deploying to Kubernetes..."
helm dependency update "$HELM_CHART_PATH"
helm upgrade --install "$HELM_RELEASE" "$HELM_CHART_PATH" -f "$HELM_VALUES_FILE"

echo "Waiting for Federated Catalogue to become ready..."
kubectl wait --for=condition=ready pod \
  -l "app.kubernetes.io/instance=${HELM_RELEASE},app.kubernetes.io/name=federated-catalogue" \
  --timeout=10m

echo "Waiting for statuslist-service..."
kubectl wait --for=condition=ready pod \
  -l "app.kubernetes.io/instance=${HELM_RELEASE},app.kubernetes.io/name=statuslist-service" \
  --timeout=5m

echo "Initializing statuslist for dev (NATS create when list is empty)..."
python3 testWallet/scripts/ensure_statuslist_for_dev.py

# Setup backend .env
cp backend/.env.dev1 backend/.env
echo "✓ .env updated from .env.dev"

# Fetch cert-chain from K8s secret
mkdir -p "$(dirname "$CERT_FILE")"
echo "Fetching cert-chain from K8s secret..."
tmp_cert_file="${CERT_FILE}.tmp"
b64_cert_chain="$(kubectl -n "$K8S_NAMESPACE" get secret "$K8S_SECRET_NAME" \
  -o "go-template={{ index .data \"$K8S_SECRET_KEY\" }}")"
if [ -z "$b64_cert_chain" ]; then
  echo "error: secret $K8S_SECRET_NAME key $K8S_SECRET_KEY is missing or empty" >&2
  exit 1
fi
printf '%s' "$b64_cert_chain" | base64 -d > "$tmp_cert_file"
if [ ! -s "$tmp_cert_file" ]; then
  rm -f "$tmp_cert_file"
  echo "error: fetched cert-chain is empty" >&2
  exit 1
fi
mv "$tmp_cert_file" "$CERT_FILE"
echo "✓ Cert-chain ready at $CERT_FILE"

# Copy .dev.env → .env so pdf-core main.go picks it up at startup.
cp "$PDF_CORE_DEV_ENV" "$PDF_CORE_ENV"

(cd "$PDF_CORE_DIR" && make start-pdf) &> /tmp/pdf-core-live.log &
PDF_CORE_PID=$!
echo "✓ pdf-core started (PID $PDF_CORE_PID) — log: /tmp/pdf-core-live.log"

# Wait for pdf-core to be ready before starting the backend (the backend
# probes /version at startup and will Fatalf if pdf-core is unreachable).
echo "Waiting for pdf-core to become ready..."
for i in $(seq 1 15); do
  if curl -sf http://localhost:8080/version >/dev/null 2>&1; then
    echo "✓ pdf-core ready"
    break
  fi
  if [ "$i" -eq 15 ]; then
    echo "error: pdf-core did not become ready in time" >&2
    exit 1
  fi
  sleep 1
done

echo ""
echo "=== Starting Vite dev server ==="
cd frontend/ClientApp
npm run dev &
VITE_PID=$!
cd ../..

sleep 2

echo ""
echo "=== Starting backend (air) ==="
cd backend
air &> /tmp/backend-live.log &
BACKEND_PID=$!

# Cleanup on exit
wait $VITE_PID 2>/dev/null || true
wait $PDF_CORE_PID 2>/dev/null || true
wait $BACKEND_PID 2>/dev/null || true
