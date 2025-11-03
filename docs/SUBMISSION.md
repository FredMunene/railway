# Submission Checklist

- [ ] `forge test --coverage` (contracts)
- [ ] `forge snapshot`
- [ ] `cd api && go test ./...`
- [ ] `docker compose up --build` (API, Postgres, Anvil, Prometheus, Grafana)
- [ ] Grafana dashboard shows intent/callback metrics updating
- [ ] DLQ drills captured + manual replay instructions exercised
- [ ] Screencast (≤10 min) demonstrating:
  1. `docker compose up` from clean state
  2. Submitting a mint intent and verifying on-chain events
  3. Simulated `/callbacks/mpesa` execution + metrics update
  4. Retry/DLQ scenario and remediation
  5. Grafana dashboard walkthrough
- [ ] ADR, Threat Model, and Runbook reviewed and committed
- [ ] `deployments.json` aligns with last deployment
- [ ] CI (GitHub Actions) green

**Notes:**
- Set `CHAIN_PRIVATE_KEY` before running compose.
- Use `POSTGRES_TEST_DSN` to run Postgres-backed idempotency tests locally.
- Screencast suggestion: OBS with window capture of terminal + Grafana.
