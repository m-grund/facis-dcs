[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](../LICENSE)

# Digital Contracting Service

An automated orchestration workspace that deploys a [Digital Contracting Service](https://github.com/eclipse-xfsc/facis/tree/main/DCS) instance to a Kubernetes cluster.

---

## Overview

The Digital Contracting Service (DCS) provides an open-source platform for creating, signing, and managing contracts digitally.
Integrated with the European Digital Identity Wallet (EUDI), it guarantees that all digital transactions are secure, legally binding, and interoperable.

Key components:
- **Multi-Contract Signing** — multi-party contract execution within a single workflow
- **Automated Workflows** — contract generation, execution, and deployment
- **Lifecycle Management** — contract monitoring with renewal/expiration alerts
- **Signature Management** — signatures linked to verifiable digital identities
- **Secure Archiving** — tamper-evident archive compliant with retention policies
- **Machine Signing** — automated signing for high-volume transactions

---

## Helm Chart

The parent chart bundles `postgresql`, `keycloak`, `hydra`, `nats`, `neo4j`, and `federated-catalogue` as optional sub-charts, each toggled via `<subchart>.enabled`.

When sub-charts are disabled, point DCS to external services via:
- `serviceDiscovery.postgresqlHost`
- `serviceDiscovery.keycloakHost`
- `serviceDiscovery.natsHost`

Routing is configured with `route.basePath` (e.g. `/tenant-a/dcs`) or explicit `paths.api` / `paths.ui` overrides.

---

## Local Development

### Prerequisites
- [Rancher Desktop](https://rancherdesktop.io/) with Kubernetes enabled (provides `kubectl`, `helm`, and NodePort forwarding to `localhost`)
- Go with [air](https://github.com/air-verse/air) (`go install github.com/air-verse/air@latest`)
- Node.js 20+
- Python 3.10+
- Goa **v3** – Installation: Follow the instructions on [Goa Quickstart](https://goa.design/docs/1-goa/quickstart/)


#### Initialize all dependencies
Run the following command in **backend** to initialize all needed dependencies:
```bash
go mod tidy
```

#### Generate Go code with Goa
Generate the required glue code under `gen/` with the Goa CLI:
```bash
goa gen digital-contracting-service/design
```

### Recommended: One-command full stack startup

From the project root:

```bash
bash dev-stack.sh
```

What this command does:
1. Runs Helm dependency update and upgrade using `deployment/helm/values.dev.yml`
2. Creates `backend/.env` from `backend/.env.dev1` if missing
3. Provisions a local SoftHSM2 token with the five DCS keys and issues the
   C2PA/PAdES x5chains for pdf-core (`scripts/hsm-provision.sh`,
   `scripts/c2pa-cert-provision.sh`)
4. Starts frontend Vite dev server
5. Starts backend with air hot reload

Stop everything with `Ctrl+C` in the same terminal.

### Manual startup (equivalent steps)

Use this if you prefer separate terminals or step-by-step debugging.

#### 1. Deploy dependencies

```bash
helm dependency update ./deployment/helm

# First setup
helm install dcs ./deployment/helm -f ./deployment/helm/values.dev.yml

# Upgrade current installation
helm upgrade dcs ./deployment/helm -f ./deployment/helm/values.dev.yml

# For uninstalling the installation
`helm uninstall dcs`
```

The dev values run the backend natively (replicaCount 0), so the PKCS#11 token
is provisioned locally by `dev-stack.sh` rather than in-cluster.

This starts all dependencies as NodePort services forwarded to `localhost`:

| Service              | Address                          |
|----------------------|----------------------------------|
| PostgreSQL           | `localhost:30432`                |
| Keycloak             | `http://localhost:30080`         |
| Hydra (public OIDC)  | `http://localhost:30444`         |
| Hydra (admin API)    | `http://localhost:30085`         |
| NATS                 | `nats://localhost:30422`         |
| Neo4j HTTP           | `http://localhost:30474`         |
| Neo4j Bolt           | `bolt://localhost:30687`         |
| Federated Catalogue  | `http://localhost:30081`         |
| IPFS Document Manager | `http://localhost:30481`        |
| IPFS Kubo RPC        | `http://localhost:30501`         |

The Keycloak `gaia-x` realm is imported automatically on first start.

> To upgrade after chart changes: `helm upgrade dcs ./deployment/helm -f ./deployment/helm/values.dev.yml`

#### 2. Prepare backend runtime config and PKCS#11 token

```bash
cp backend/.env.dev1 backend/.env
bash scripts/hsm-provision.sh "$HOME/.dcs/softhsm-8991" dcs 1234 12345678
```

This provisions the SoftHSM2 token with the five DCS keys; the backend opens it
via the `PKCS11_*` / `SOFTHSM2_CONF` variables in `.env`. `dev-stack.sh` performs
this step automatically.

#### 3. Run backend and frontend

Terminal 1:

```bash
cd backend && air
```

Terminal 2:

```bash
cd frontend/ClientApp
npm install
npm run dev
```

The backend listens on `http://localhost:8991`.

The Vite dev server starts at `http://localhost:5173` and proxies `/api` requests to the backend automatically.

### 4. Sign in with the demo wallet

```bash
python3 testWallet/demo_wallet.py
```

---

## BDD Tests

BDD scenarios live in `features/` at the project root. Tests are run against a full stack in an ephemeral [kind](https://kind.sigs.k8s.io/) cluster.

### Prerequisites
- `kind` — `go install sigs.k8s.io/kind@v0.23.0` or see [kind releases](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- `kubectl` and `helm`
- Docker (to build the DCS image)
- Python 3.10+

### Run locally

These steps are primarily for working on tests. Start the deployment, run the tests as often as you want, stop the deployment.
```bash
# 1. Start the environment
make -C tests/bdd kind_up

# 2. Run tests
# All
make -C tests/bdd run_bdd_kind_once
# File/Folder
make -C tests/bdd run_bdd_kind_once F=features/<PATH>

# 3. Stop the environment and reset
make -C tests/bdd kind_down
```

These step is for deploy and auto-run all tests a single time
```bash
# Build DCS image, spin up kind cluster, deploy via Helm, run all scenarios
make -C tests/bdd run_bdd_kind_ci
```

This single command:
1. Builds the DCS Docker image (`digital-contracting-service:bdd`)
2. Creates a kind cluster named `dcs-bdd`
3. Loads the image into the cluster
4. Deploys the full stack via `deployment/helm` with `values.bdd.yml`
5. Port-forwards DCS and Keycloak into the cluster network
6. Runs all `features/**/*.feature` scenarios with behave

Tear down the cluster afterwards:
```bash
make -C tests/bdd kind_delete
```

### Run against an already-deployed Helm release

If you have a release running (e.g. via Rancher Desktop):

```bash
make -C tests/bdd run_bdd_helm_dev \
  K8S_NAMESPACE=default \
  HELM_RELEASE=dcs
```

### CI

The `bdd-kind.yml` GitHub Actions workflow runs:

```yaml
make -C tests/bdd run_bdd_kind_ci
```

JUnit reports are published as check annotations and uploaded as workflow artifacts.

---

## Production Deployment

### Signing keys (PKCS#11) and the C2PA x5chain

Every DCS private key lives in a PKCS#11 token (DCS-IR-HI-01). For dev, staging
and CI the chart co-deploys a SoftHSM2 software token and provisions it in-cluster
(`pkcs11.provisioning.enabled=true`): a hook Job runs `scripts/hsm-provision.sh`
(token + five ECDSA P-256 keys) and `scripts/c2pa-cert-provision.sh` (the C2PA
x5chain bound to the `dcs-c2pa` key), publishing the x5chain as a Secret that
pdf-core mounts. The backend waits for the token via an initContainer, then
opens it using `PKCS11_MODULE_PATH` / `PKCS11_TOKEN_LABEL` / `PKCS11_PIN`.

SoftHSM2 is a software token and is NOT a production HSM. For production set
`pkcs11.provisioning.enabled=false` and point `pkcs11` at a real external PKCS#11
module whose token already holds the keys:

```yaml
pkcs11:
  modulePath: /usr/lib/<vendor>/libpkcs11.so
  tokenLabel: dcs
  pinSecretRef:
    name: dcs-hsm-pin
    key: PKCS11_PIN
  provisioning:
    enabled: false
```

### Hydra
- Enable `hydra.enabled` and set `hydra.config.selfIssuerURL` to the public issuer URL
- Register `dcs-client` redirect URIs via `hydra.clients` (see `values.dev.yml`):
  - **Valid Redirect URIs**: `https://<domain>/<path>/api/auth/callback`
  - **Valid Post Logout Redirect URIs**: `https://<domain>/<path>/api/auth/logout-complete`

### Keycloak (Federated Catalogue only)
- FC integration uses `fcKeycloak.realmURL` / `FC_KEYCLOAK_REALM_URL`

### TLS
- Use certificates from a trusted Certificate Authority
- Recommend [cert-manager](https://cert-manager.io/) for automatic renewal

### Values
Override the following at minimum:

```yaml
hydra:
  enabled: true
  config:
    selfIssuerURL: "https://hydra.example.com"
  clients:
    - client_id: dcs-client
      client_secret: "<secret>"
      redirect_uris: ["https://example.com/dcs/api/auth/callback"]
      post_logout_redirect_uris: ["https://example.com/dcs/api/auth/logout-complete"]

fcKeycloak:
  realmURL: "https://keycloak.example.com/realms/gaia-x"

route:
  basePath: "/dcs"
```

---

## License

Apache License 2.0. See [LICENSE](../LICENSE).

## TSA (Timestamp Authority)

DCS uses an RFC 3161 Timestamp Authority to cryptographically prove that a document or contract existed at a specific point in time. The timestamp is unforgeable and independent of DCS itself.

- Timestamps are requested via ORCE, which forwards requests to the upstream TSA provider
- The TSR (timestamp response) is verified by DCS using the TSA's CA certificate embedded in the binary (`backend/internal/base/tsa/certs/tsa.crt`)
- Stored TSRs can be re-verified at any time against the original data to prove it has not been altered

### Switching TSA providers

1. Update the TSA flow in ORCE to point to the new provider
2. Replace `backend/internal/base/tsa/certs/tsa.crt` with the new provider's CA certificate (PEM format) and rebuild the backend
