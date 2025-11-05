# FiatRails Video Walkthrough Checklist

Use the following script to capture a **≤10‑minute** recording that covers the full system. Each section lists the exact commands to run and what to highlight on screen.

---

## 0. Prerequisites (prep off camera)

```bash
# Make sure the repo is clean and dependencies are available
docker --version
docker compose --version
foundryup --version          # or `forge --version`
cast --version
jq --version
node --version
python3 --version

# (optional) Clean existing containers/images/volumes
docker compose down -v
rm -rf contracts/deployments.json deployments.json
```

---

## 1. Bring Up the Stack From a Clean State

```bash
docker compose up --build
```

* Start the recording once the command runs.
* Narrate the container startup sequence; wait until Prometheus, Grafana, Postgres, Anvil, and the API are all healthy.
* Leave the terminal visible until you see “API listening on :3000”.

---

## 2. End-to-End Demo Flow (Mint Intent → Callback → Mint Executed)

Open a new terminal tab (keep the `docker compose up` logs visible in the background for atmosphere).

```bash
# Recreate deployments and walk through the flow
./scripts/deploy-and-demo.sh
```

The script automates:

1. Contract deployment on Anvil.
2. Funding and approving the test user.
3. Marking the user compliant.
4. Submitting the signed `POST /api/v1/mint-intents`.
5. Triggering the signed `POST /api/v1/callbacks/mpesa`.
6. Verifying the intent and on-chain balances.

* Narrate the key outputs:
  * Mint intent ID and transaction hash.
  * Callback status (`processed`).
  * Final intent status = `Executed` and updated country-token balance.

---

## 3. Grafana Dashboard (Metrics)

* Visit [http://localhost:3001](http://localhost:3001) (login admin/admin).
* Open the FiatRails dashboard (left menu → “Dashboards” → “FiatRails Overview”).
* Highlight changing widgets:
  * Mint intents total.
  * Callback success metrics.
  * RPC latency panels.
* Optional: trigger another mint via `scripts/deploy-and-demo.sh` (or just steps 3–5 within it) to show live updates.

---

## 4. Simulate RPC Failure to Show Retry/Backoff

1. Keep the API running.
2. Temporarily block Anvil:

   ```bash
   docker compose exec anvil pkill anvil
   ```

   (or `docker compose stop anvil` if pkill is unavailable).

3. Trigger an M-PESA callback while Anvil is down:

   ```bash
   # Reuse the last intent ID or create a new one with the demo script (Step 2).
   curl -X POST http://localhost:3000/api/v1/callbacks/mpesa \
     -H "Content-Type: application/json" \
     -H "X-Request-Timestamp: $(date +%s)" \
     -H "X-Mpesa-Signature: <computed signature>" \
     -d '{"intentId":"<INTENT_ID>","txRef":"<TX_REF>","userAddress":"<USER>","amount":"1000000000000000000"}'
   ```

4. In the `docker compose up` terminal, point out log lines from the API showing retry attempts with increasing backoff (look for messages mentioning “retry” or “backoff”).
5. Restart Anvil:

   ```bash
   docker compose up -d anvil
   ```

6. Re-run the callback to show it succeeds once the RPC endpoint is healthy.

---

## 5. DLQ Item Example

When the callback fails after max retries (Step 4), the API drops a DLQ file (default path: `api/dlq/` or the path configured via `DLQPath`).

Showcase:

```bash
ls api/dlq
cat api/dlq/<latest-file>.json
```

Point out the recorded intent, failure reason, and timestamps.

---

## 6. Wrap-Up

* Return to the Grafana dashboard to show metrics reflecting the failed attempt(s) and recovery.
* Close with the `docker compose down` command:

```bash
docker compose down
```

This keeps the recording tidy and demonstrates a full cycle from clean start to clean teardown.

---

**Tips for the Recording**

- Use a window manager that lets viewers see both terminal and browser side-by-side.
- Narrate each major step (“Now we’re submitting the signed mint intent…”, “Here’s the retry/backoff in the logs…”).
- Keep an eye on the running time—aim for 8–9 minutes to stay well under the 10‑minute limit.
