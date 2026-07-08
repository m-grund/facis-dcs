# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Frontend (`frontend/ClientApp/`)
```bash
npm run dev          # Vite dev server (port 5173, falls back to 5174)
npm run build        # vue-tsc type-check + Vite build
npm run lint         # ESLint
npm run lint:fix     # ESLint with auto-fix
npm run format       # Prettier
npm run format:check # Prettier check (used in CI)
npx vue-tsc --noEmit # TypeScript-only check (no build output)
```

### Backend (`backend/`)
```bash
go run ./cmd/dcs                               # Start HTTP server on :8991
air                                            # Dev mode with hot reload (.air.toml)
go test -v ./...                               # Run all tests
goa gen digital-contracting-service/design     # Regenerate transport/types (required after any design/ change)
go mod tidy                                    # Required before every commit
./bin/golangci-lint run                        # Static analysis (auto-installed by pre-commit hook)
```

### Full Stack
```bash
npm install          # Project root — installs Husky pre-commit hooks (run once)
bash dev-stack.sh    # One-command local stack: Helm (K8s), backend air, frontend Vite
bash dev-stack2.sh   # Optional second instance ("instance B") for the two-instance inter-org
                     # demo (Workstream C1-C3, docs/anforderung.md): its own Helm release
                     # ("dcs2", deployment/helm/values.dev2.yml), backend on :8992
                     # (backend/.env.dev2, built and run directly rather than via `air`),
                     # frontend on :5174 (`npm run dev-dcs2`). Run dev-stack.sh first — pdf-core
                     # is shared between both instances.
```

## Architecture Overview

### Full-Stack Layout
Single Go binary serves both the backend API (under `/api`) and the static Vue.js frontend (under `/ui/`). In dev mode Vite proxies `/api` → `http://localhost:8991`.

```
backend/           Go service (Goa v3, port 8991)
frontend/ClientApp Vue 3 + Vite + Pinia (port 5173)
deployment/        Docker multi-stage build, Helm chart
tests/bdd/         Python-based BDD integration tests
```

### Backend — Goa Code-First API Design

The backend follows a strict layering enforced by Goa:

1. **`backend/design/`** — Goa DSL files define all API endpoints, types, and errors. This is the authoritative source of truth for the HTTP interface. *Never edit `gen/` directly.*
2. **`backend/gen/`** — Goa-generated HTTP handlers and types. Regenerate with `goa gen digital-contracting-service/design` after any design change.
3. **`backend/internal/`** — Business logic. Each domain has its own package mirroring the design file (e.g., `templaterepository/`, `contractworkflowengine/`). Follows a repository pattern with DB access in `internal/*/db/`.

When adding a new endpoint: write the DSL in `design/`, run `goa gen`, then implement the method in `internal/`.

### Frontend — Module/Service/Store Separation

Three distinct layers in `frontend/ClientApp/src/`:

- **Services** (`services/`) — Axios calls to the backend. Interfaces are declared in `models/services/`; implementations live in `services/`. Services are thin: they call `http.ts` or `auth-http.ts` and return typed responses.
- **Stores** (`stores/`) — Seven domain Pinia stores for cross-page state (auth, tokens, contracts, templates, error, nav, filters). These are *not* editor stores — they hold list/cache/session state.
- **Module stores** (`modules/template-repository/store/`) — Feature-scoped Pinia stores for the editor. The primary one is `dcsDraftStore`, which holds the active JSON-LD document being edited.

### Template Editor — JSON-LD as Single Source of Truth

The template builder uses `dcsDraftStore` (not `templateDraftStore`, which is legacy). The store state *is* the JSON-LD document (`dcs:ContractTemplate`):

- **`dcs:sections`** — flat ordered array of `DcsClause | DcsTextBlock`. No tree hierarchy.
- **`odrl:policy`** (assembled as getter) — ODRL rules (`OdrlDuty`, `OdrlPermission`, `OdrlProhibition`). Types are concrete, not the abstract `odrl:Rule`.
- **`templateDocument` getter** — assembles the full `DcsTemplateData` JSON-LD object from flat state, ready to POST to the API.
- **`loadDocument(rawDoc, meta)`** — handles both new JSON-LD and legacy formats gracefully via `isDcsTemplateData()` guard.

Type definitions: `src/models/dcs-jsonld.ts`. SLA/ODRL vocabulary: `modules/template-repository/utils/sla-ontology-catalog.ts` (single source for action IRIs, metric IRIs, operators, units — no hardcoded lists in components).

### Routing & Auth

`src/router/router.ts` — all routes are guarded by `beforeEach`. Guards check:
1. Token validity (refreshes if expired)
2. Role membership (`TEMPLATE_CREATOR`, `CONTRACT_CREATOR`, etc.)

Unauthenticated users are redirected to `/login`. Role mismatch redirects to `/unauthorized`.

### Vite Path Aliases (frontend)

| Alias | Resolves to |
|-------|-------------|
| `@/` | `src/` |
| `@core/` | `src/core/` |
| `@template-repository/` | `src/modules/template-repository/` |

### Environment Variables

**Frontend** — Vite reads `DCS_` prefixed env vars (see `frontend/ClientApp/.env.development`):
- `DCS_API_PATH` — API base path (default `/api`)
- `DCS_API_TARGET` — Backend URL for proxy (default `http://localhost:8991`)
- `DCS_UI_PATH` — Frontend base path (default `/ui/`)

**Backend** — configure via `backend/.env.dev` (copied to `.env` by `dev-stack.sh`). Key groups: `DATABASE_URL`, `HYDRA_*` (OIDC), `NATS_URL`, `CRYPTO_PROVIDER_*`, `IPFS_*`, `TSA_URL`.

### Pre-commit Hooks

Husky runs `lint-staged` on staged frontend files and `golangci-lint` + `go mod tidy` check on staged backend files. The linter is auto-installed at `./bin/golangci-lint` if missing. A commit that leaves `go.mod`/`go.sum` un-tidy will be blocked.

### Docker Build

Multi-stage `deployment/docker/Dockerfile`:
1. Node 22 stage builds the Vue frontend
2. Go 1.25 stage runs `goa gen` then builds the binary
3. Debian slim runtime serves both at port 8991

`npm run build:docker` skips `vue-tsc` for faster Docker builds.
