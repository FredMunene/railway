# THREAT MODEL

**Candidate:** Munene  
**Date:** 2025-11-02  
**Version:** 1.0

---

## On-Chain Threats

### Reentrancy in MintEscrow
- **Risk:** Token transfers or mint callbacks could re-enter `executeMint`.
- **Mitigation:** Escrow uses the checks-effects-interactions pattern, marks intents executed before external calls, and relies on OZ `ReentrancyGuard` semantics.

### Replay / Duplicate Execution
- **Risk:** Attackers replay `executeMint` with the same `intentId`.
- **Mitigation:** Intent status changes to `Executed` before minting; subsequent calls revert with `IntentAlreadyExecuted`.

### Role Escalation / Privileged Function Abuse
- **Risk:** Unauthorized actors call `updateUser` or `executeMint`.
- **Mitigation:** AccessControl roles (ADMIN, EXECUTOR, COMPLIANCE) gated with OZ `onlyRole`. Initial role grants occur in scripts/tests; production expects multisig governance.

### Upgrade Bricking
- **Risk:** Malicious upgrade breaks storage.
- **Mitigation:** `_authorizeUpgrade` requires `UPGRADER_ROLE`. `_disableInitializers()` prevents re-initialization. Runbook covers rollback.

---

## Off-Chain API Threats

### HMAC Forgery
- **Risk:** Attackers craft requests to `/mint-intents` or `/callbacks/mpesa`.
- **Mitigation:** Shared-secret HMAC (`X-Request-Signature`, `X-Mpesa-Signature`) with timestamp freshness window. Secrets stored in env vars; runbook covers rotation.

### Idempotency Bypass
- **Risk:** Replay leads to duplicate mints/refunds.
- **Mitigation:** PostgreSQL idempotency table enforces unique keys; callbacks key off `txRef`.

### Retry Storm / Thundering Herd
- **Risk:** Repeated retries overwhelm RPC or API.
- **Mitigation:** Seed-driven exponential backoff with DLQ after 6 attempts. Metrics/alerts monitor retry volume.

### Queue Poisoning / DLQ Flood
- **Risk:** Malicious payloads fill DLQ.
- **Mitigation:** DLQ entries include timestamp/payload/error; dashboard surface depth. Runbook outlines manual remediation.

### Database Compromise
- **Risk:** Attackers tamper with idempotency responses.
- **Mitigation:** Postgres access restricted to API network. Responses signed via HMAC on the client side; forged payloads still fail signature check.

---

## Operational Threats

### Secret Leakage
- **Risk:** HMAC salts or private keys leak via logs or repo.
- **Mitigation:** Config loads via env; no secrets in Git. Runbook covers rotation.

### Key Rotation Downtime
- **Risk:** Rotation causes signing mismatches.
- **Mitigation:** Runbook prescribes dual-secret deployment (accept old + new) and staged rollout.

### Infrastructure Failure (RPC / DB)
- **Risk:** RPC or Postgres downtime halts processing.
- **Mitigation:** Health endpoint reports status; Prometheus + Grafana dashboards signal outages. DLQ captures failed callbacks for replay.

### CI/CD Supply Chain
- **Risk:** Malicious dependencies/lints.
- **Mitigation:** CI pins Foundry and uses official `actions/setup-go`. Dockerfile uses multi-stage builds with distroless runtime.

---

## Residual Risks & Future Work
- Add jitter to retry backoff to further reduce synchronous retry spikes.
- Implement automatic DLQ replay tooling once manual process is battle-tested.
- Consider hardware security module (HSM) for production key storage.

**Signed:** Munene  
**Date:** 2025-11-02
