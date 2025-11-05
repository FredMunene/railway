# Architecture Decision Records

**Candidate:** Munene  
**Date:** 2025-11-02  
**Version:** 1.0

---

## ADR-001: Upgradeability Pattern

### Context

FiatRails must be able to evolve compliance controls after deployment. We reviewed UUPS, Transparent Proxy, Beacon, and Diamond patterns for the one contract that needs runtime upgrades (`ComplianceManager`). Operational contracts that hold user funds (`MintEscrow`, token contracts, `UserRegistry`) stay immutable to keep the surface for governance mistakes small.

### Decision

**Pattern Chosen:** UUPS (Universal Upgradeable Proxy Standard)

### Rationale

- Leverages OpenZeppelin’s audited `UUPSUpgradeable` mixin with minimal boilerplate.
- Keeps proxy storage lean and gas costs lower than Transparent proxies.
- Gives explicit hooks (`_authorizeUpgrade`) to restrict upgrade authority to governance.

**Pros:**
- Minimal proxy storage and gas overhead.
- Reuses well-understood OZ components already in the codebase.
- Upgrade authorization lives in the implementation contract, simplifying tooling.

**Cons:**
- Any bug in `_authorizeUpgrade` could brick the proxy.
- Requires strict initializer hygiene to avoid leaving implementation mutable.

### Trade-offs Considered

| Pattern     | Gas Cost | Admin Key Risk                         | Complexity | Chosen? |
|-------------|----------|----------------------------------------|------------|---------|
| UUPS        | Low      | Medium (logic contract holds gate)     | Medium     | Yes     |
| Transparent | Medium   | Low (proxy admin contract)             | Low        | No      |
| Beacon      | Medium   | Medium                                 | High       | No      |

### How Misuse is Prevented

- Constructors call `_disableInitializers()` so logic contracts cannot be reinitialized.
- `_authorizeUpgrade` enforces the `UPGRADER_ROLE`; only governance-approved addresses can upgrade.
- The initializer wires roles once and requires a non-zero admin and registry, preventing half-initialized deployments.

```solidity
constructor() {
    _disableInitializers();
}

function _authorizeUpgrade(address newImplementation)
    internal
    override
    onlyRole(UPGRADER_ROLE)
{}
```

### References

- contracts/src/ComplianceManager.sol:12  
- contracts/src/IComplianceManager.sol:10  
- docs/RUNBOOK.md:88

---

## ADR-002: Event Schema Design

### Context

Our indexer and reporting pipelines reconstruct compliance state and mint history directly from logs. Events must support lookups by user, intent, and geography without bloating gas usage.

### Decision

**Indexed Fields Strategy:** Index identifiers (`intentId`, `user`, `countryCode`) and leave wide or mostly unique values (`amount`, `txRef`) unindexed.

```solidity
event MintExecuted(
    bytes32 indexed intentId,
    address indexed user,
    uint256 amount,
    bytes32 indexed countryCode,
    bytes32 txRef
);
```

### Rationale

**Why These Fields Indexed:**
- `intentId`: correlate on-chain executions to API requests and callbacks.
- `user`: power user-centric dashboards and AML investigations.
- `countryCode`: aggregate per token/country for treasury reconciliation.

**Why These NOT Indexed:**
- `amount`: wide numeric range; filters are rare and the topic would add ~375 gas.
- `txRef`: unique per payment. We retain it in the data payload for audits but indexing offers little reuse.

### Indexer Requirements

1. Pull all mint executions and refunds for a given wallet quickly.
2. Aggregate mint volume and refund rate per `countryCode`.
3. Track risk-score audit trail via `UserRiskUpdated` and `AttestationRecorded`.

### Trade-offs

- Three indexed topics keep gas reasonable while supporting the key query dimensions.
- Leaving `amount` and `txRef` in the data payload avoids extra topic fan-out and keeps filters simple.

---

## ADR-003: Idempotency Strategy

### Context

HTTP clients and webhook providers retry aggressively. We must guarantee that duplicate submissions do not double-mint or issue extra refunds, even when callbacks race or infrastructure restarts mid-flight.

### Decision

- **Deduplication Key:**  
  - `/api/v1/mint-intents`: caller-provided `X-Idempotency-Key`.  
  - `/api/v1/callbacks/mpesa`: namespaced payment reference `mpesa:<txRef>`.
- **Storage:** PostgreSQL table `idempotency_records`; if the DSN is empty the API falls back to a JSON-backed `FileStore` for local dev.
- **TTL:** 24 hours, derived from `timeouts.idempotencyWindowSeconds` (86,400 seconds) in `seed.json`.

```sql
CREATE TABLE IF NOT EXISTS idempotency_records (
    key TEXT PRIMARY KEY,
    status_code INT NOT NULL,
    response BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
```

### Rationale

- Using the client-provided header keeps `/mint-intents` compatible with standard API clients (Stripe style). Namespacing M-PESA references avoids collisions with user-provided keys.
- PostgreSQL gives durable, ACID semantics that survive process restarts and support multiple API replicas.
- The 24-hour TTL covers all retry windows required by partner SLAs while bounding table growth.

### Edge Cases Handled

1. **Concurrent requests with same key:** Reads hit the store before work; once the first response is saved, later requests reuse the cached payload. PostgreSQL’s `ON CONFLICT` upsert keeps the row consistent.
2. **Expired keys:** `Get` checks `expires_at`; expired rows return `nil` and are pruned asynchronously (PostgreSQL) or immediately (file store) before returning.
3. **Database failure:** If Postgres cannot be reached at boot, the API logs the failure and initializes the file-backed store so retries remain safe, albeit without cross-instance sharing.

### Alternatives Considered

- **Redis with TTL:** rejected because the local stack already ships with Postgres and Redis would add another service plus persistence concerns.
- **In-memory cache:** rejected; would lose dedup history on restart and offers no protection in multi-instance deployments.

---

## ADR-004: Key Management

### Context

The system relies on multiple secrets: HMAC salts for clients, the webhook secret from M-PESA, and the Ethereum signer private key. These must be rotated without exposing them or requiring code changes.

### Decision

**Storage:** Environment variables injected via Docker Compose locally and secrets manager (GitHub Actions secrets / future Vault) in production. `config.Load` reads them at startup and never persists them.

**Rotation Strategy:** Update the secret source (secret manager or `.env`), restart the API container, and verify new signatures with a health probe.

### Rationale

**For Production:**
- Works with any secret manager that can project env vars (AWS Secrets Manager, Vault, SSM) without coupling code to a specific provider.
- Keeps secrets out of the filesystem and process arguments; rotates via rolling restarts.

**For This Trial:**
- `.env` is ignored by Git (`.gitignore`) and only loaded locally.
- Docker Compose examples document required variables; CI pulls them from repository secrets.

### Rotation Procedure (from RUNBOOK)

```bash
# Update secret store and redeploy API
docker compose up -d api
```

1. Generate new secret and update the relevant env var (`HMAC_SALT`, `MPESA_WEBHOOK_SECRET`, `CHAIN_PRIVATE_KEY`).
2. Restart the API (Compose, systemd, or CI deployment).
3. Send a signed canary request with the new secret to confirm success, then revoke the old secret from clients.

### Security Considerations

- Secrets never logged: verification middleware returns generic 401s and does not print secrets.
- Secrets never in Git: `.env` is ignored and docs instruct using env injection; seed values are examples only.
- Secrets scoped per environment: Compose/CI use different env files; production can supply unique salts and keys through the orchestrator.

---

## ADR-005: Retry and Backoff Parameters

### Context

RPC calls to execute mints can fail transiently (nonce errors, network blips). The callback handler must retry without overloading the node or duplicating side effects.

### Decision

**From seed.json:**
```json
{
  "retry": {
    "maxAttempts": 6,
    "initialBackoffMs": 751,
    "maxBackoffMs": 30000,
    "backoffMultiplier": 2
  }
}
```

**Backoff Formula:** `min(backoff * multiplier^(attempt-1), maxBackoff)`

**Jitter:** Not yet applied; tracked as a follow-up to add ±10% jitter once we integrate distributed queues.

### Rationale

- **Max attempts (6):** Covers roughly 2.5 minutes of retries, long enough for brief RPC hiccups without holding webhooks indefinitely.
- **Initial backoff (751 ms):** Aligns with seed config; keeps immediate retries snappy while spacing them out exponentially.
- **Multiplier (2) and cap (30s):** Doubles wait each time but caps to avoid multi-minute delays; prevents thundering herd when many callbacks arrive.

### Dead-Letter Queue Trigger

After all attempts fail, the payload is serialized to `dlq/<timestamp>-<txRef>.json` for manual remediation.

```json
{
  "timestamp": "2025-11-02T08:15:30Z",
  "payload": {
    "intentId": "0x...",
    "txRef": "MPESA123",
    "userAddress": "0x...",
    "amount": "1000000000000000000"
  },
  "error": "execute mint tx: nonce too low"
}
```

### Recovery

- DLQ items are manually reviewed via the runbook, corrected, and replayed with `curl`.
- Grafana dashboard tracks DLQ depth; alerts fire when depth > 0 (planned).
- No automatic replays yet—operators decide whether to retry, refund, or escalate.

---

## ADR-006: Database Choice

### Context

We need durable storage for idempotency responses and potentially for DLQ metadata. The system should run locally without heavy setup but be production-ready.

### Decision

**Database:** PostgreSQL (via Docker Compose) with a file-backed fallback for single-node development.

### Rationale

- PostgreSQL brings ACID guarantees, row-level locks, and rich observability to share state across API replicas.
- Compose already provisions Postgres; leveraging it avoids managing another service while staying production-aligned.

**Schema:**
```sql
CREATE TABLE IF NOT EXISTS idempotency_records (
    key TEXT PRIMARY KEY,
    status_code INT NOT NULL,
    response BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
```

### Alternatives Considered

| Database | Pros                         | Cons                             | Chosen? |
|----------|------------------------------|----------------------------------|---------|
| PostgreSQL | Durable, concurrent writers | Requires managed service         | Yes     |
| SQLite   | Embedded, zero external deps | Single-writer bottleneck; no HA | No      |
| Redis    | Fast, built-in TTL           | Persistence optional, extra ops | No      |

---

## Summary Table

| Decision        | Choice                                  | Key Trade-off                          |
|-----------------|-----------------------------------------|----------------------------------------|
| Upgradeability  | UUPS for `ComplianceManager`            | Gas efficiency vs. upgrade gate safety |
| Event Indexing  | intentId/user/country indexed           | Gas savings vs. query flexibility      |
| Idempotency     | Header key + PostgreSQL store (24h TTL) | Operational cost vs. reliability       |
| Key Management  | Environment variables + documented ops  | Ease of use vs. enforced rotation      |
| Retry Logic     | 6-attempt exponential backoff (no jitter yet) | Latency vs. resilience            |
| Database        | PostgreSQL with dev file fallback       | Durability vs. setup complexity        |

---

## Notes for Reviewers

- Retry jitter is pending; tracked to add randomness before multi-node rollout.
- MintEscrow currently relies on governance for upgrades; if requirements change we can wrap it in a proxy with the same pattern.
- `SeedConstants` capture the seed values at build time; regenerate if the seed changes to keep contracts aligned.

---

**Signed:** Munene  
**Date:** 2025-11-02

