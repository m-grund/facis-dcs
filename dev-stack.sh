#!/bin/bash
# Full dev stack orchestration: Helm → Vite → Air
# Run from project root: bash dev-stack.sh
# Press Ctrl+C to stop everything

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

echo "=== Setting up dev environment ==="

# Setup Helm
echo "Updating Helm dependencies and deploying to Kubernetes..."
helm dependency update "$HELM_CHART_PATH"
helm install "$HELM_RELEASE" "$HELM_CHART_PATH" -f "$HELM_VALUES_FILE"

echo "Waiting for Federated Catalogue to become ready..."
kubectl wait --for=condition=ready pod \
  -l "app.kubernetes.io/instance=${HELM_RELEASE},app.kubernetes.io/name=federated-catalogue" \
  --timeout=10m

# Setup .env
if [ ! -f backend/.env ]; then
  cp backend/.env.dev backend/.env
  echo "✓ .env file created from .env.dev"
fi

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
air

# Cleanup on exit
wait $VITE_PID 2>/dev/null || true
