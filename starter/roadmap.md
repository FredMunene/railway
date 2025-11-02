Phase 1: Setup and Foundation (The First Hour)
Before writing any code, get your environment and configuration in order. This prevents headaches later.

Understand Your Unique Seed:

Open seed.json. This file is your unique configuration for the entire project.
Take note of your chainId, countryCode, stablecoin.symbol, and compliance.maxRiskScore. These values must be used throughout your implementation. This is a key part of the evaluation.
Install Core Dependencies:

As the README.md suggests, install Foundry for smart contract development and testing.
Install the dependencies for your chosen API language (e.g., npm install for a Node.js project in the /api directory).
Initialize Your Project Structure:

Run forge init contracts to set up the Foundry project inside the /contracts directory.
Initialize your API project in the /api directory (e.g., npm init).
Create your initial Git commit: git commit -m "chore: initial project setup". Commit frequently from now on to show your thought process.
Phase 2: On-Chain Development (Solidity & Foundry)
This is the heart of the protocol. Build the contracts from simplest to most complex.

Simple Tokens (USDStablecoin.sol, CountryToken.sol):

Create these two ERC20 token contracts. You can use OpenZeppelin's standard ERC20.sol contract.
For CountryToken.sol, add the ERC20Burnable and AccessControl extensions. The MINTER_ROLE will be granted to the MintEscrow contract later. Its symbol should come from your seed.json.
For USDStablecoin.sol, you can add a public mint function to pre-fund test accounts.
UserRegistry.sol:

This contract acts as your on-chain compliance database.
Define a struct UserData { uint8 riskScore; bytes32 attestationHash; }.
Use a mapping: mapping(address => UserData) private _users;.
Implement an updateUser function, guarded by onlyRole(COMPLIANCE_OFFICER_ROLE). This function should emit the UserRiskUpdated and/or AttestationRecorded events.
Create a public/external view function like isCompliant(address user) that other contracts can call. This function will check the user's risk score against the maxRiskScore from your seed.json.
ComplianceManager.sol (The Admin Contract):

This will be your upgradeable "entry point" for administration.
Inherit from OpenZeppelin's UUPSUpgradeable, Pausable, and AccessControl.
In the initialize function, grant the initial roles (ADMIN, UPGRADER, COMPLIANCE_OFFICER) to the deployer address.
Crucially, implement the _authorizeUpgrade function as required by UUPS to ensure only an address with UPGRADER_ROLE can perform an upgrade.
Start filling out ADR-001 in docs/ADR.md to justify your choice of UUPS. Explain the pros (gas savings) and cons (risk of a bad implementation bricking the contract) and how _authorizeUpgrade mitigates this.
MintEscrow.sol (The Core Logic):

This is the most complex contract. It will implement the IMintEscrow.sol interface you were given.
It should hold addresses for the UserRegistry, the USDStablecoin, and each CountryToken it can mint (e.g., in a mapping mapping(bytes32 => address)).
submitIntent function:
It should calculate a unique intentId (e.g., keccak256(abi.encodePacked(msg.sender, amount, countryCode, txRef, block.timestamp))).
Check that this intent doesn't already exist.
Pull the amount of USDStablecoin from msg.sender using transferFrom. The user must have approved the MintEscrow contract first.
Store the MintIntent struct in a mapping.
Emit the MintIntentSubmitted event.
executeMint function:
This function should be protected so only your API's authorized address can call it.
Checks-Effects-Interactions Pattern is critical here:
Checks: Fetch the intent. Verify its status is Pending. Call userRegistry.isCompliant(intent.user).
Effects: Immediately update the intent's status to Executed. This prevents re-entrancy and double-execution.
Interactions: Call mint() on the correct CountryToken contract, sending tokens to the user.
Emit the MintExecuted event.
refundIntent function: Similar flow, but transfers the stablecoin back to the user and sets the status to Refunded.
Write Tests (Continuously):

For every function you write, add a corresponding test in the contracts/test/ directory.
Unit Tests: Test individual functions in isolation (e.g., does updateUser correctly change the risk score?).
Integration Tests: Test the full flow. A key test would be:
Admin grants roles.
User approves stablecoin for MintEscrow.
User calls submitIntent.
API (simulated by test) calls executeMint.
Assert that the user received the CountryToken and their stablecoin balance decreased.
Fuzz/Invariant Tests: Use Foundry's powerful fuzzing to test for edge cases (e.g., what happens with amount = 0 or amount = type(uint256).max?). An invariant test could be "the total supply of a CountryToken never exceeds the total stablecoins held by the escrow."
Phase 3: Off-Chain API Development
Now, build the bridge between the world and your contracts. Use the api/openapi.yaml as your strict guide.

Setup API Server & Endpoints:

Create a basic server with endpoints for /mint-intents, /callbacks/mpesa, /health, and /metrics.
Implement /mint-intents:

HMAC Verification: Create middleware to verify the X-Request-Signature and X-Request-Timestamp.
Idempotency: Before processing, check the X-Idempotency-Key against your database (Postgres/SQLite). If it exists, return the stored response. If not, begin processing.
On-Chain Call: Use a library like Ethers.js or Viem to call the submitIntent function on your deployed MintEscrow contract.
Store Idempotency: After a successful submission, store the intentId and a 201 Created status in your idempotency table, associated with the key.
Implement /callbacks/mpesa:

This is the most resilience-critical endpoint.
Security: Verify the X-Mpesa-Signature and timestamp freshness immediately.
Idempotency: Use the txRef from the M-PESA payload as the deduplication key.
Core Logic with Retries:
Wrap the call to escrow.executeMint(intentId) in a retry loop.
Use the exponential backoff parameters from your seed.json. Add jitter to prevent thundering herds.
Only retry on specific, transient RPC errors (e.g., timeout, nonce too low, network error), not on contract reverts (like "UserNotCompliant").
Dead-Letter Queue (DLQ): If the call fails after all maxAttempts, write the entire callback payload and the final error to a DLQ file (e.g., /dlq/{timestamp}-{txRef}.json). This is your safety net.
Implement /health and /metrics:

/health: Should check its database connection and RPC connection (e.g., by calling eth_blockNumber).
/metrics: Use a Prometheus client library. Instrument your code to increment counters (fiatrails_mint_intents_total) and record durations (fiatrails_rpc_duration_seconds) for every important operation.
Phase 4: Operations and Documentation
This is what separates a script from a production service.

Dockerize Everything:

Write a Dockerfile for your API.
Create a docker-compose.yml that orchestrates all services: your API, a Postgres DB, an Anvil node for local testing, Prometheus, and Grafana. Use the provided ops/ files as a starting point. Ensure docker compose up works flawlessly.
Configure Observability:

Configure prometheus.yml to scrape the /metrics endpoint of your API container.
Create a simple Grafana dashboard (.json file in grafana/dashboards) to visualize the key metrics from the README: RPC error rate, p95 latency, DLQ depth, and mint success rate.
Write Your Documentation (The Brain of the Project):

ADR.md: Fill out all the sections. Justify your choices for UUPS, event indexing (which fields need to be searchable?), idempotency key storage, and so on. This shows you think about trade-offs.
THREAT_MODEL.md: Think like an attacker. Go through the list and explain your mitigations. For T-001: Reentrancy, point to your use of the Checks-Effects-Interactions pattern. For T-102: Replay Attacks, describe your timestamp and txRef checks.
RUNBOOK.md: This is for the on-call engineer (you!). Write clear, step-by-step instructions for the scenarios listed. The "HMAC Secret Rotation" with zero downtime is a classic senior-level task. The "Contract Upgrade Rollback" procedure should be precise and tested.
Phase 5: Final Polish and Submission
CI/CD (.github/workflows/ci.yml):

Create a GitHub Actions workflow that runs on every push. It should:
Install dependencies.
Run forge test --coverage and forge snapshot.
Run a linter on your code.
Build your API's Docker image to ensure it works.
Create deployments.json:

Write a deployment script (e.g., in /scripts) that deploys all your contracts to your local Anvil node and saves the addresses to a deployments.json file at the root.
Record the Screencast:

Keep it under 10 minutes.
Start with a clean slate (docker compose down -v).
Run docker compose up.
Use curl or a simple script to hit /mint-intents, then /callbacks/mpesa.
Show the transaction on a block explorer (Anvil's output).
Show the metrics updating in your Grafana dashboard.
Demonstrate a retry by temporarily stopping the Anvil container and sending a callback. Show the logs and the DLQ item being created.
