# Architecture Decision Records

**Candidate:** Munene  
**Date:** 2025-11-02  
**Version:** 1.0

---

## ADR-001: Upgradeability Pattern

### Context

MintEscrow and ComplianceManager must be upgradeable post-deployment. Options considered: UUPS proxies, Transparent proxies, Beacon proxies, and Diamond pattern. The project already relies on OpenZeppelin libraries.

### Decision

**Pattern Chosen:** UUPS (Universal Upgradeable Proxy Standard)

### Rationale

UUPS keeps deployment cost low and piggybacks on OpenZeppelin’s audited implementation. It preserves explicit upgrade authorization (`_authorizeUpgrade`) while avoiding the extra proxy admin indirection of the Transparent pattern.

**Pros:**
- Lowest gas overhead among common proxy options.
- Reuses well-maintained OZ components with clear hooks for authorization.
- Keeps proxy storage minimal, making audits simpler.

**Cons:**
- Upgrade auth lives in the implementation; a faulty `_authorizeUpgrade` can brick the system.
- Requires strict initializer hygiene to avoid leaving the logic contract mutable.

### Trade-offs Considered

| Pattern      | Gas Cost | Admin Key Risk              | Complexity | Chosen? |
|--------------|----------|-----------------------------|------------|---------|
| UUPS         | Low      | Medium (logic contract gate)| Medium     | ✅       |
| Transparent  | Medium   | Low (proxy gate)            | Low        | ❌       |
| Beacon       | Medium   | Medium                      | High       | ❌       |

### How Misuse is Prevented

- Constructors call `_disableInitializers()` to freeze the implementation.
- `_authorizeUpgrade` enforces `UPGRADER_ROLE` so only governance-approved actors can upgrade.
- OZ storage gaps keep future variable additions safe.

### References

- OpenZeppelin UUPS guide – https://docs.openzeppelin.com/contracts/4.x/api/proxy#UUPSUpgradeable
- Internal contract implementation in `contracts/src/ComplianceManager.sol`

---

## ADR-002: Event Schema Design

### Context

Off-chain services rely on events to reconcile intents, compliance changes, and mint execution. Events need to be filterable by intent, user, and geography without incurring unnecessary gas.

### Decision

**Indexed Fields Strategy:**
- `intentId`, `user`, and `countryCode` are indexed.
- Scalar values (`amount`, `txRef`) remain unindexed.

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

**Indexed:**
- `intentId` → direct lookup for reconciliation and callback correlation.
- `user` → user-centric dashboards and AML checks.
- `countryCode` → business reporting per token/country.

**Not Indexed:**
- `amount` → wide numeric range; filters are rare, keeping it unindexed saves gas.
- `txRef` → unique per payment; indexing adds cost with little reuse.

### Indexer Requirements

1. Fetch all mint executions for a user quickly.
2. Aggregate mint volume by country/token.
3. Track risk score adjustments (`UserRiskUpdated`) per user over time.

### Trade-offs

- Gas vs. queryability: three indexed topics strike a balance.
- Topic fan-out kept manageable for log-based analytics.

---

## ADR-003: Idempotency Strategy

### Context

Both REST clients and M-PESA callbacks can retry, and the API must avoid duplicate on-chain effects.

### Decision

- **Deduplication Key:**
  - `/mint-intents`: trust the caller-provided `X-Idempotency-Key`.
  - `/callbacks/mpesa`: namespace the payment reference as `mpesa:${txRef}`.
- **Storage:** PostgreSQL table (`idempotency_records`). File-backed JSON store is used only in dev when Postgres is absent.
- **TTL:** 24 hours (`timeouts.idempotencyWindowSeconds`).

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

Postgres provides durable, ACID semantics and can be shared across API replicas. Keys expire after 24h, keeping the table small while covering typical retry windows.

---

## ADR-004: Secret Management Strategy

### Context

HMAC salts, private keys, and webhook secrets must never leak. Compose, CI, and production environments need a consistent handling plan.

### Decision

- Secrets delivered via environment variables (`CHAIN_PRIVATE_KEY`, `HMAC` salts, DB credentials).
- `.env` or secret managers populate variables locally; CI uses GitHub Actions secrets.
- API reads secrets at boot and never logs them.

### Implications

- No secrets committed to Git.
- Compose and CI documentation explicitly reference required variables.
- Rotation handled through environment updates + process restarts (see RUNBOOK).

---

## ADR-005: Retry and Backoff Parameters

### Context

RPC calls fail intermittently. The callback handler should retry without overwhelming the node or duplicating mints.

### Decision

- Seed-driven values:
  - `maxAttempts`: 6
  - `initialBackoffMs`: 751
  - `maxBackoffMs`: 30000
  - `backoffMultiplier`: 2
- Formula: `min(initialBackoff * multiplier^(attempt-1), maxBackoff)`; jitter (+/-10%) to be added in a future iteration.
- After exhausting attempts, payload is written to DLQ for manual triage.

### Rationale

Six attempts with exponential backoff cover transient issues (~2.5 minutes) without blocking the queue indefinitely. DLQ ensures eventual operator visibility.

---

## ADR-006: Database Choice

### Context

Idempotency and DLQ entries require durable storage. Options considered: PostgreSQL, SQLite, Redis.

### Decision

- **Database:** PostgreSQL (compose) with optional JSON file fallback when the DB is unavailable.

### Rationale

- ACID guarantees and concurrent writers support multiple API instances.
- Mature ecosystem (migrations, observability, backups).
- Compose already provisions Postgres; no extra infra cost.

| Database | Pros | Cons | Chosen? |
|----------|------|------|---------|
| PostgreSQL | Durable, concurrent, easy to observe | Requires service management | ✅ |
| SQLite | Embedded, zero setup | Single-writer bottleneck | ❌ |
| Redis | Fast, TTL built-in | Persistence optional, extra ops work | ❌ |

---

## Summary Table

| Decision        | Choice                         | Key Trade-off                       |
|-----------------|--------------------------------|-------------------------------------|
| Upgradeability  | UUPS                           | Gas efficiency vs. upgrade auth hub |
| Event Indexing  | intentId/user/country indexed | Gas vs. query flexibility           |
| Idempotency     | Postgres + header keys        | Ops complexity vs. reliability      |
| Secrets         | Env vars + documented rotation| Security vs. operational overhead   |
| Retry Strategy  | Seed-driven exponential backoff| Latency vs. resilience              |
| Database        | PostgreSQL                     | Operational cost vs. durability     |

---

## Notes for Reviewers

- Compose stack exercises the full flow (Anvil + Postgres + Prometheus/Grafana).
- CI builds Docker image, runs Foundry + Go tests, and lints Solidity.
- Jitter is not yet implemented; tracked in backlog for future resilience work.

**Signed:** Munene  
**Date:** 2025-11-02
