# Implemented Features

## Smart Contracts
- **ComplianceManager (contracts/src/ComplianceManager.sol)**  
  - *What:* Upgradeable admin facade that routes compliance actions to `UserRegistry`, enforces pausing, and guards upgrades via role-based access.  
  - *How:* Uses OpenZeppelin UUPS mixins with `_disableInitializers()` in the constructor and `_authorizeUpgrade` restricted to `UPGRADER_ROLE`. Admins can pause/unpause operations and set the registry address.  
  - *Why:* Keeps compliance logic evolvable without touching value-holding escrows and ensures only designated governance keys can deploy new logic.

- **UserRegistry (contracts/src/UserRegistry.sol)**  
  - *What:* Stores per-user risk scores and attestation metadata, emitting audit events on change.  
  - *How:* Access-controlled (`MANAGER_ROLE`) `updateUser` enforces risk range, attestation requirements from seed-derived constants, and records timestamps. `isCompliant` checks max risk and attestation freshness.  
  - *Why:* Centralizes policy enforcement so both on-chain escrows and off-chain services rely on a single source of truth derived from `seed.json`.

- **MintEscrow (contracts/src/MintEscrow.sol)**  
  - *What:* Accepts stablecoin deposits, tracks mint intents, enforces daily limits, and mints country tokens after compliance checks.  
  - *How:* Uses intent structs keyed by keccak hash, SafeERC20 for transfers, executor role gating, and per-day mint accounting. Emits `MintIntentSubmitted`, `MintExecuted`, and `MintRefunded`; refunds return stablecoin to the user.  
  - *Why:* Guarantees idempotent minting and strong compliance gating before releasing country tokens, mirroring production flows.

- **CountryToken & USDStablecoin**  
  - *What:* Seed-driven ERC20s (`CountryToken.sol`, `USDStablecoin.sol`) providing minter access to `MintEscrow` and open mint for tests.  
  - *How:* Metadata constants originate from `SeedConstants`; `CountryToken` restricts minting to `MINTER_ROLE`.  
  - *Why:* Aligns on-chain assets with candidate-specific configuration and enables local demos without external tokens.

## Off-Chain API
- **Configuration Loading (api/internal/config/config.go)**  
  - *What:* Aggregates `seed.json`, `deployments.json`, and environment variables into a single runtime config.  
  - *How:* Computes retry durations, idempotency TTL, DLQ paths, and optional database DSN; exposes structured config to the server.  
  - *Why:* Keeps API behavior deterministic across environments and ensures seed values propagate consistently.

- **Mint Intent Endpoint**  
  - *What:* `POST /api/v1/mint-intents` accepts HMAC-signed requests and forwards them to the escrow client.  
  - *How:* Middleware verifies timestamped signatures; the handler validates payloads, submits on-chain via `escrow.Client`, caches the response in the idempotency store, and updates Prometheus counters.  
  - *Why:* Provides a production-style API surface that prevents replay attacks and guarantees idempotent intent submission.

- **M-PESA Callback Endpoint**  
  - *What:* `POST /api/v1/callbacks/mpesa` processes signed webhooks, retries on-chain execution, and writes DLQ entries on failure.  
  - *How:* Uses dedicated verifier for `X-Mpesa-Signature`, namespaced idempotency keys (`mpesa:<txRef>`), exponential backoff from seed values, and DLQ persistence to disk.  
  - *Why:* Reflects real-world webhook handling where network/RPC instability is common and operators must triage failures.

- **Health & Metrics**  
  - *What:* `/api/v1/health` reports service, RPC, DB status; `/api/v1/metrics` exposes Prometheus counters/gauges.  
  - *How:* Health probes ping optional Postgres/eth clients, include latency, and aggregate DLQ depth; metrics track mint outcomes, callbacks, retries, and queue backlog.  
  - *Why:* Supports observability expectations for production-readiness and ties into the provided Grafana dashboard.

## Idempotency & Storage
- **PostgreSQL Store with File Fallback (api/internal/idempotency)**  
  - *What:* Durable storage for previous responses keyed by idempotency tokens.  
  - *How:* Postgres variant ensures table creation, upsert semantics, and TTL pruning; file-backed implementation supports local dev when DSN absent.  
  - *Why:* Maintains consistent behavior across retries and survives process restarts while keeping local setup friction low.

## Observability & Operations
- **Metrics Registry (api/internal/server/metrics.go)**  
  - *What:* Central registry for business and reliability metrics used by Prometheus/Grafana.  
  - *How:* CounterVecs record mint/callback outcomes and retry results; gauge reflects DLQ depth.  
  - *Why:* Enables at-a-glance insight into system health and supports future alerting rules.

- **Runbook & ADRs (docs/)**  
  - *What:* Completed ADRs, runbook, threat model templates, and deployment notes specific to this build.  
  - *How:* `docs/ADR.md`/`ADR.template.md` capture trade-offs; `RUNBOOK.md` details rotations, incident response, DLQ handling.  
  - *Why:* Demonstrates operational maturity and documents decisions for reviewers and future operators.

- **Docker & Compose Scaffolding (ops/)**  
  - *What:* Compose-ready Prometheus/Grafana configs, alert stubs, and environment defaults.  
  - *How:* `ops/prometheus.yml` and Grafana dashboards mirror metrics emitted by the API.  
  - *Why:* Provides turnkey observability stack aligned with emitted telemetry.

## Demo & Tooling
- **Deploy & Demo Script (scripts/deploy-and-demo.sh)**  
  - *What:* End-to-end automation that deploys contracts, configures roles, runs the API, submits intents, triggers callbacks, and validates balances.  
  - *How:* Uses Foundry, `cast`, curl, and Python helpers to create HMAC signatures; prints contract addresses and verification output.  
  - *Why:* Gives reviewers a reproducible walkthrough proving the entire pipeline functions as designed without manual steps.

- **Seed Constants (contracts/src/utils/SeedConstants.sol)**  
  - *What:* Hard-coded constants derived from `seed.json` for contract deployments.  
  - *How:* Encodes token metadata, compliance thresholds, chain info, and mint limits into Solidity library.  
  - *Why:* Ensures deterministic contract behavior and keeps on-chain logic synchronized with candidate-specific configuration.

## Security & Safeguards
- **HMAC Verification (api/internal/hmacauth)**  
  - *What:* Shared middleware validating signatures with configurable headers and skew windows.  
  - *How:* Recomputes SHA-256 HMAC over timestamp + body, rejects stale requests, and restores the request body for downstream handlers.  
  - *Why:* Prevents replay and tampering for both user-facing and webhook endpoints.

- **Access Control & Roles**  
  - *What:* All contracts use explicit roles (`ADMIN`, `EXECUTOR`, `COMPLIANCE_OFFICER`, `MINTER`) with OpenZeppelin `AccessControl`.  
  - *How:* Roles granted at initialization; admin-only methods configure registry, stablecoin, executors, and tokens.  
  - *Why:* Minimizes blast radius of compromised keys and segregates duties (e.g., executor vs. admin).

These components combine to deliver a production-shaped slice of FiatRails: deterministic configuration from `seed.json`, secure intent capture, upgradeable compliance controls, resilient webhook processing, measurable observability, and a scripted demo that proves the flow end-to-end.

