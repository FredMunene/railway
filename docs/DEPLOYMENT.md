# Deployment Script

## Prerequisites
- Foundry installed (`forge --version`).
- An RPC endpoint for the target network (Anvil or remote).
- Exported private key for the deployer address: `DEPLOYER_PRIVATE_KEY`.

Optional overrides:
- `ADMIN_ADDRESS` – defaults to the deployer if unset.
- `EXECUTOR_ADDRESS` – defaults to the deployer if unset.
- `DEPLOYMENTS_PATH` – output location for the generated JSON file (defaults to `deployments.json`).

## Running the script
```bash
export DEPLOYER_PRIVATE_KEY=0xabc123...
export RPC_URL=http://localhost:8545

forge script script/DeployFiatRails.s.sol:DeployFiatRails \
  --rpc-url $RPC_URL \
  --broadcast \
  -vvvv
```

The script deploys the USD stablecoin, country token, user registry, compliance manager (via proxy), and mint escrow. It then wires the required roles:
- Grants the compliance manager permission to update the registry.
- Sets up the mint escrow with the stablecoin, registry, country token, and executor.
- Grants the mint escrow the `MINTER_ROLE` on the country token.

## Output
After a successful run the script writes a JSON summary (default `deployments.json`):
```json
{
  "chainId": 31430,
  "deployer": "0x...",
  "admin": "0x...",
  "executor": "0x...",
  "contracts": {
    "USDStablecoin": "0x...",
    "CountryToken": "0x...",
    "UserRegistry": "0x...",
    "ComplianceManager": "0x...",
    "MintEscrow": "0x..."
  }
}
```

Adjust `DEPLOYMENTS_PATH` if you need to store multiple environment snapshots (e.g., `out/deployments/anvil.json`). Use the generated file as the single source of truth for off-chain services.
