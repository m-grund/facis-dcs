# ADR: Why OCM-W NATS services are not used for VC signing in DCS

## Context

DCS embeds a W3C Verifiable Credential (a `ContractLifecycleCredential`) inside each C2PA manifest it generates. The VC must carry an `Ed25519Signature2020` LD proof so that validators can verify the binding between the contract artefact and the lifecycle event.

When designing this, we evaluated whether to route the signing call through the Eclipse XFSC Organisational Credential Manager W-Stack (OCM-W), which DCS may co-deploy and communicate over NATS.

We researched the OCM-W NATS API via the [`nats-message-library`](https://github.com/eclipse-xfsc/nats-message-library) — the canonical message-contract library used by all OCM-W services.

## OCM-W services evaluated

| Service | NATS subject | What it actually does |
|---|---|---|
| Credential issuance service | `issuance.request` | Starts an **OID4VCI** issuance flow — returns a `CredentialOffer` (a URL/token) that a *wallet holder* redeems asynchronously to pull their credential. |
| Signer service | `signer.signToken` | Signs a **JWT** (header + payload bytes) and returns a compact JWT token. |
| Storage service | `storage.service.credential` | Stores an already-signed credential in the wallet. |
| Well-known / registration | `wellknown.issuer.registration` | Registers issuer metadata for OID4VCI discovery. |

## Why none of these fit

**Issuance (`issuance.request`)** is designed for the OID4VCI wallet-pull protocol: a human or automated agent initiates an offer, and a remote wallet later redeems it over HTTP. The flow is asynchronous, multi-step, and requires a holder DID on the other end. DCS needs synchronous in-process signing to embed the proof *before* the PDF is returned to the caller — there is no wallet, no holder, and no round-trip tolerable.

**Signer (`signer.signToken`)** produces a compact JWT, not an LD proof. `Ed25519Signature2020` requires URDNA2015 canonicalization of the document and proof options, SHA-256 hashing of both, concatenation, Ed25519 signing, and multibase base58btc encoding of the result. That entire construction is absent from the signer service's API; it only accepts raw header/payload bytes and returns a token.

In short, the OCM-W stack is built around the OID4VCI credential-issuance protocol for issuing credentials *to* wallet holders. DCS's requirement — synchronously sign an in-memory JSON-LD document and embed the result in a binary container — is orthogonal to that protocol.

## Decision

DCS signs VCs via the **HSM directly** (PKCS#11, `backend/internal/base/hsm`) and constructs an `ecdsa-rdfc-2019` Data Integrity proof locally (`provenance.NewHSMVCSigner`):

1. URDNA2015-canonicalize the proof options and the VC document (`piprate/json-gold`)
2. SHA-256 hash each
3. Send the concatenation to the PKCS#11-held ECDSA P-256 key for signing
4. Set `proof.proofValue` from the resulting signature

This supersedes an earlier iteration of this decision that called a Vault
transit engine (`crypto-provider` Helm subchart) directly instead of the
HSM — see "Findings Collected During Implementation" below, which is kept
as the historical record of that iteration. Workstream A (PKI
consolidation) replaced Vault-backed key custody with PKCS#11/SoftHSM2
throughout DCS, including VC signing: **one key-custody mechanism, not
two** (see ADR-1). The `crypto-provider` Helm chart no longer exists in
this repository.

## Consequences

- No OCM-W dependency for VC signing, and no Vault dependency either —
  VCs, C2PA claim signatures, and PAdES signatures now all resolve to the
  same PKCS#11 key-custody layer (ADR-1).
- If DCS later needs to *issue credentials to wallet holders* via OID4VCI
  (e.g. issuing a signed contract summary to a participant's wallet), the
  OCM-W issuance service would be the right integration point for that
  separate feature — that reasoning is unaffected by the Vault→HSM signer
  change.

## Findings Collected During Implementation (historical — Vault-era)

The findings below were recorded against the Vault-transit-engine iteration
of this decision, before the PKCS#11 migration (ADR-1) replaced it. Kept
for historical record; none of it applies to the current HSM-backed signer.

### 1. Status list liveness/probes had two valid paths in different components

- DCS startup originally probed `STATUSLIST_SERVICE_URL + "/health"`.
- The deployed status list service exposes `GET /v1/metrics/health` as readiness/liveness endpoint.
- Resulting startup failure seen in logs:
	- `status list service not reachable at http://localhost:30821`
	- `Get "http://localhost:30821/health": read ... connection reset by peer`
- Fix applied:
	- DCS startup now probes both `/health` and `/v1/metrics/health`.
	- Helm statuslist probes switched to `/v1/metrics/health`.

### 2. Status list chart env naming mismatch caused service instability

- Status list service expects `STATUSLIST_*` env variables.
- Chart previously mixed in non-matching names for several settings.
- Fix applied:
	- Helm env names aligned to `STATUSLIST_*` consistently.

### 3. Signer `credential/proof` endpoint proved incompatible for DCS VC flow

Direct matrix tests against `POST /v1/credential/proof` showed non-viable behavior:

- Missing `group` returns `400 "group" is missing from body`.
- `ed25519` key with `ed25519signature2020` still failed in this deployment (`unsupported key type: ed25519`).
- `ecdsa-p256` key with `jsonwebsignature2020` failed (`unsupported key type: ecdsa-p256`).
- RSA keys failed similarly (`unsupported key type: rsa-2048/rsa-4096`).
- Mismatched key/suite combinations produced `Key doesnt match to signature type. Must be ed key.`

This ruled out using signer-managed `credential/proof` for lifecycle VC generation in the current stack.

### 4. `POST /v1/sign` has strict request contract

- The signer requires `group` in the JSON body for `/v1/sign` too.
- Omitting `group` produced runtime error:
	- `sign returned 400: {"message":"\"group\" is missing from body"}`
- Fix applied:
	- DCS sign request body always includes `"group":""`.

### 5. Dedicated VC key provisioning became mandatory after local proof switch

- Local VC proof generation uses a dedicated Ed25519 transit key (`dcs-vc-signing-key`).
- Runtime 500s occurred when the key did not exist in Vault:
	- signer log: `signing key not found` for `transit/sign/dcs-vc-signing-key`
- Fix applied:
	- Helm crypto-provider vault-init hook provisions both:
		- C2PA key (`dcs-signing-key-p256`, `ecdsa-p256`)
		- VC key (`dcs-vc-signing-key`, `ed25519`)
	- DCS receives `CRYPTO_PROVIDER_VC_KEY` explicitly.

### 6. JSON-LD payload strictness required explicit context and URI-safe subject IDs

- Earlier payloads failed compaction/signing checks when context/term mapping was incomplete.
- Non-URI `credentialSubject.id` values failed strict validators.
- Fix applied:
	- Inline DCS context terms included in lifecycle VC (`contract_id`, `file_hash`, `status`, `reason`, `effective_at`).
	- Subject IDs normalized to URI form (`did/urn` passthrough, otherwise deterministic `urn:dcs:subject:<sha256>`).

### 7. Dev configuration should be explicit, not fallback-driven

- Multiple hidden defaults masked config drift and delayed diagnosis.
- Fix applied:
	- Dev env and dev Helm values now declare explicit VC/C2PA signing values.
	- Runtime signing code now fails fast when URL/namespace/key/issuer are missing instead of silently falling back.
