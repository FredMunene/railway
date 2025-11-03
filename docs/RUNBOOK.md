# RUNBOOK

**Candidate:** Munene  
**Date:** 2025-11-02  
**Version:** 1.0

---

## 1. Service Overview
- **API:** Go HTTP service exposing `/mint-intents`, `/callbacks/mpesa`, `/health`, `/metrics`.
- **Dependencies:** Ethereum RPC (Anvil / L2 RPC), PostgreSQL, Prometheus, Grafana.
- **Secrets:** HMAC salts, M-PESA webhook secret, `CHAIN_PRIVATE_KEY`, DB credentials.

Compose environment:
- API – `fiatrails-api` container (port 3000)
- Postgres – port 5432 (mapped to host 55432 if adjusted)
- Anvil – port 8545
- Prometheus – port 9090
- Grafana – port 3001

---

## 2. Dashboards & Alerts
- **Grafana:** `FiatRails Overview` dashboard (Grafana → Dashboards → Browse → FiatRails Overview).
  - Panels: mint intent totals, callback totals, retry rate, DLQ depth.
- **Prometheus Alerts:** (to be integrated) – set alert rules on DLQ depth > 0 and retry rate spikes.

---

## 3. Operational Tasks

### 3.1 Deploy / Redeploy API
```bash
export CHAIN_PRIVATE_KEY=<hex key>
docker compose up --build -d         # local stack
# or in CI/CD, run actions workflow (ci.yml)
```

### 3.2 Rotate HMAC / Webhook Secrets
1. Generate new secret values.
2. Update environment (.env, secrets manager, GitHub secrets).
3. Restart API containers.
4. Verify `/health` returns `healthy` and signatures work (test request with new secret).
5. Delete old secrets after clients are updated.

### 3.3 Rotate `CHAIN_PRIVATE_KEY`
1. Pause callback processing (`docker compose stop api` or set maintenance flag).
2. Generate new key, update `CHAIN_PRIVATE_KEY` env.
3. Restart API.
4. Verify new address has necessary roles/ETH funding.
5. Resume traffic.

### 3.4 Handle DLQ Entries
1. Check DLQ depth (Grafana or filesystem in `dlq/`).
2. Inspect JSON payload (`cat dlq/<timestamp>-<txRef>.json`).
3. Determine root cause (RPC outage vs. business rule).
4. After fix, replay manually:
   ```bash
   curl -X POST http://localhost:3000/api/v1/callbacks/mpesa \
     -H 'X-Mpesa-Signature: ...' \
     -H 'X-Request-Timestamp: ...' \
     -d @payload.json
   ```
5. Remove processed DLQ file.

### 3.5 Database Maintenance
- Run `psql postgres://fiatrails:fiatrails@localhost:55432/fiatrails` for manual queries.
- Vacuum/cleanup idempotency table periodically (`DELETE` older than TTL).

---

## 4. Incident Response

### 4.1 RPC Down
- Symptom: `/health` shows `rpc.connected=false`.
- Actions:
  1. Check Anvil or upstream node logs.
  2. Switch `CHAIN_RPC_URL` to backup node if available.
  3. Restart API with new RPC endpoint.
  4. Replay DLQ entries after recovery.

### 4.2 Database Down
- Symptom: `/health` shows `database.connected=false`; API logs `postgres store error`.
- Actions:
  1. Restart/Postgres service.
  2. Confirm connectivity (`psql`).
  3. Without DB, API falls back to file store only on restart—avoid long-term operation without DB.

### 4.3 HMAC / Signature Failures
- Symptom: 401 responses from mint/callback endpoints.
- Actions:
  1. Verify secrets in env.
  2. Check client timestamp skew.
  3. Rotate secrets if compromised.

### 4.4 Upgrade Rollback
1. Deploy previous Docker image (`docker compose up -d --build` with prior tag) or revert to previous git commit + redeploy.
2. Contracts: follow Foundry script `DeployFiatRails.s.sol` with old implementation addresses if necessary (requires careful storage compatibility).

---

## 5. Operational Contact & Logging
- Logs accessible via `docker compose logs api`.
- Prometheus/Grafana logs accessible through their containers.
- For production, integrate alerting with email/Slack once thresholds defined.

**Signed:** Munene  
**Date:** 2025-11-02
