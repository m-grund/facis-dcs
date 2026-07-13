# Digital Contracting Service (DCS)

[![Behavior Tests](https://github.com/eclipse-xfsc/facis-dcs/actions/workflows/bdd-kind.yml/badge.svg)](https://github.com/eclipse-xfsc/facis-dcs/actions/workflows/bdd-kind.yml)

The **Digital Contracting Service** provides an **open-source platform** for creating, signing, and managing contracts digitally.  
Integrated with the **European Digital Identity Wallet (EUDI)**, it guarantees that all digital transactions are secure, legally binding, and interoperable.  
DCS allows organizations to streamline business processes, reduce paperwork, and ensure **compliance with eIDAS 2.0 regulations**, while fostering trust across federated partners.

**The detailed specifications for the Digital Contracting Service (DCS) can be found: [SRS_FACIS_DCS](https://github.com/eclipse-xfsc/facis/tree/main/DCS/specification/SRS_FACIS_DCS.pdf).**

## Development Setup
Run `npm install` in the **project root**. This installs **Husky** and registers the pre-commit hooks. If you skip this, you cannot commit code, and your Pull Request will fail the CI pipeline.
```bash
# Execute this in the root directory
npm install
```

## DCS Backend
- You can find instruction for the DCS Backend here: [DCS Backend](./backend/README.md)

## Development Quick Start (Rancher Desktop + Helm)
This repository uses Helm-managed dependencies in Kubernetes for local development.

1. Start Rancher Desktop with Kubernetes enabled.
2. From project root, run:

```bash
bash dev-stack.sh
```

This sets up Helm dependencies, prepares backend env and C2PA cert-chain, then starts frontend (Vite) and backend (air).

For step-by-step manual commands and troubleshooting, see [deployment/README.md](./deployment/README.md#local-development).