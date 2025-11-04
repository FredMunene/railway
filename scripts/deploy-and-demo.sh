#!/bin/bash

# FiatRails Demo Script - One-shot deployment and demonstration
# This script deploys contracts and runs a demo flow

set -e

DEFAULT_DEPLOYER_PK="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
DEFAULT_EXECUTOR_PK="0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
DEFAULT_USER_PK="0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a"

export DEPLOYER_PRIVATE_KEY="${DEPLOYER_PRIVATE_KEY:-$DEFAULT_DEPLOYER_PK}"
export CHAIN_PRIVATE_KEY="${CHAIN_PRIVATE_KEY:-$DEFAULT_EXECUTOR_PK}"
export TEST_USER_PRIVATE_KEY="${TEST_USER_PRIVATE_KEY:-$DEFAULT_USER_PK}"
export ETH_RPC_URL="${ETH_RPC_URL:-http://localhost:8545}"

echo "════════════════════════════════════════════════════════"
echo "       FiatRails Production Trial - Deploy & Demo"
echo "════════════════════════════════════════════════════════"
echo ""

# Load seed.json
CANDIDATE_ID=$(cat seed.json | jq -r .candidateId)
CHAIN_ID=$(cat seed.json | jq -r .chain.chainId)
COUNTRY_CODE=$(cat seed.json | jq -r .tokens.country.countryCode)
HMAC_SECRET=$(cat seed.json | jq -r .secrets.hmacSalt)
MPESA_SECRET=$(cat seed.json | jq -r .secrets.mpesaWebhookSecret)

echo "Candidate: $CANDIDATE_ID"
echo "Chain ID: $CHAIN_ID"
echo "Country: $COUNTRY_CODE"
echo ""

# Check prerequisites
echo "📋 Checking prerequisites..."

if ! command -v docker &> /dev/null; then
    echo "❌ Docker not found. Please install Docker."
    exit 1
fi

if ! command -v forge &> /dev/null; then
    echo "❌ Foundry not found. Please install Foundry."
    exit 1
fi

if ! command -v node &> /dev/null; then
    echo "❌ Node.js not found. Please install Node.js."
    exit 1
fi

echo "✅ Prerequisites met"
echo ""

# Start services
echo "🚀 Starting services with docker compose..."
# docker compose up -d

echo ""
echo "⏳ Waiting for services to be healthy..."
sleep 10

# Wait for Anvil
until curl -s http://localhost:8545 -X POST -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' > /dev/null; do
    echo "   Waiting for Anvil..."
    sleep 2
done
echo "✅ Anvil ready"

# Wait for API
until curl -s http://localhost:3000/api/v1/health > /dev/null; do
    echo "   Waiting for API..."
    sleep 2
done
echo "✅ API ready"

echo ""
echo "📝 Deploying contracts..."
cd contracts

# Deploy (candidate should implement this)
forge script script/Deploy.s.sol --rpc-url http://localhost:8545 --broadcast

echo "✅ Contracts deployed"
cd ..

DEPLOYMENTS_FILE="contracts/deployments.json"
if [ ! -f "$DEPLOYMENTS_FILE" ]; then
    echo "❌ Deployments file not found at $DEPLOYMENTS_FILE"
    exit 1
fi

USD_STABLECOIN=$(jq -r '.contracts.USDStablecoin' "$DEPLOYMENTS_FILE")
COUNTRY_TOKEN=$(jq -r '.contracts.CountryToken' "$DEPLOYMENTS_FILE")
USER_REGISTRY=$(jq -r '.contracts.UserRegistry' "$DEPLOYMENTS_FILE")
COMPLIANCE_MANAGER=$(jq -r '.contracts.ComplianceManager' "$DEPLOYMENTS_FILE")
MINT_ESCROW=$(jq -r '.contracts.MintEscrow' "$DEPLOYMENTS_FILE")

echo ""
echo "🔗 Contract addresses"
echo "  USDStablecoin:     $USD_STABLECOIN"
echo "  CountryToken:      $COUNTRY_TOKEN"
echo "  UserRegistry:      $USER_REGISTRY"
echo "  ComplianceManager: $COMPLIANCE_MANAGER"
echo "  MintEscrow:        $MINT_ESCROW"

echo ""
echo "🧪 Running demo flow..."

TEST_USER_ADDRESS=$(cast wallet address --private-key "$TEST_USER_PRIVATE_KEY" | tr -d '\r\n')
DEPLOYER_ADDRESS=$(cast wallet address --private-key "$DEPLOYER_PRIVATE_KEY" | tr -d '\r\n')
EXECUTOR_ADDRESS=$(cast wallet address --private-key "$CHAIN_PRIVATE_KEY" | tr -d '\r\n')

for name in TEST_USER_ADDRESS DEPLOYER_ADDRESS EXECUTOR_ADDRESS; do
    value="${!name}"
    if [ -z "$value" ]; then
        echo "❌ Failed to derive $name (output was empty)"
        exit 1
    fi
done

MINT_AMOUNT_WEI="${MINT_AMOUNT_WEI:-1000000000000000000}"
FUND_AMOUNT_WEI="${FUND_AMOUNT_WEI:-100000000000000000000}"
RISK_SCORE="${RISK_SCORE:-10}"
TX_REF="mpesa-$(date +%s)"

echo "1️⃣ Setup test user (${TEST_USER_ADDRESS})"
cast send "$USD_STABLECOIN" "mint(address,uint256)" "$TEST_USER_ADDRESS" "$FUND_AMOUNT_WEI" \
    --private-key "$DEPLOYER_PRIVATE_KEY" --rpc-url "$ETH_RPC_URL" >/dev/null
cast send "$USD_STABLECOIN" "approve(address,uint256)" "$MINT_ESCROW" "$MINT_AMOUNT_WEI" \
    --private-key "$TEST_USER_PRIVATE_KEY" --rpc-url "$ETH_RPC_URL" >/dev/null
echo "   → Stablecoin minted and escrow approval granted"

echo "2️⃣ Set compliance for user"
ATTESTATION_HASH=$(cast keccak "kyc:$TEST_USER_ADDRESS:$TX_REF")
ATTESTATION_TYPE=$(python3 - <<'PY'
value = "KYC".encode()
print("0x" + value.hex().ljust(64, "0"))
PY
)
cast send "$COMPLIANCE_MANAGER" \
    "updateUser(address,uint8,bytes32,bytes32)" \
    "$TEST_USER_ADDRESS" "$RISK_SCORE" "$ATTESTATION_HASH" "$ATTESTATION_TYPE" \
    --private-key "$DEPLOYER_PRIVATE_KEY" --rpc-url "$ETH_RPC_URL" >/dev/null
echo "   → User marked compliant"

echo "3️⃣ Submit mint intent"
MINT_HMAC_OUTPUT=$(
python3 - "$TEST_USER_ADDRESS" "$MINT_AMOUNT_WEI" "$COUNTRY_CODE" "$TX_REF" "$HMAC_SECRET" <<'PY'
import sys, json, hmac, hashlib, time
user, amount, country, tx, secret = sys.argv[1:]
if not all([user, amount, country, tx, secret]):
    print(f"missing parameter: {user=} {amount=} {country=} {tx=} {secret=}", file=sys.stderr)
    sys.exit(1)
payload = json.dumps({
    "userAddress": user,
    "amount": amount,
    "countryCode": country,
    "txRef": tx,
}, separators=(',', ':'))
timestamp = str(int(time.time()))
signature = hmac.new(secret.encode(), timestamp.encode() + payload.encode(), hashlib.sha256).hexdigest()
print(timestamp)
print(payload)
print(signature)
PY
)
IFS=$'\n' read -r MINT_TS MINT_BODY MINT_SIGNATURE <<<"$MINT_HMAC_OUTPUT"
echo "   debug mint payload: $MINT_BODY"
if [ -z "$MINT_SIGNATURE" ]; then
    echo "❌ Failed to compute HMAC signature for mint intent"
    exit 1
fi
INTENT_RESPONSE=$(curl --fail --silent --show-error \
    -X POST "http://localhost:3000/api/v1/mint-intents" \
    -H "Content-Type: application/json" \
    -H "X-Request-Timestamp: $MINT_TS" \
    -H "X-Request-Signature: $MINT_SIGNATURE" \
    -H "X-Idempotency-Key: demo-$TX_REF" \
    -d "$MINT_BODY")
INTENT_ID=$(echo "$INTENT_RESPONSE" | jq -r '.intentId')
INTENT_TX=$(echo "$INTENT_RESPONSE" | jq -r '.txHash')
if [ -z "$INTENT_ID" ] || [ "$INTENT_ID" = "null" ]; then
    echo "❌ Mint intent failed: $INTENT_RESPONSE"
    exit 1
fi
echo "   → Intent submitted (ID: $INTENT_ID, tx: $INTENT_TX)"

echo "4️⃣ Trigger M-PESA callback"
IFS=$'\n' read -r CALLBACK_TS CALLBACK_BODY CALLBACK_SIGNATURE <<<"$(
python3 - "$INTENT_ID" "$TX_REF" "$TEST_USER_ADDRESS" "$MINT_AMOUNT_WEI" "$MPESA_SECRET" <<'PY'
import sys, json, hmac, hashlib, time
intent, tx, user, amount, secret = sys.argv[1:]
payload = json.dumps({
    "intentId": intent,
    "txRef": tx,
    "userAddress": user,
    "amount": amount,
}, separators=(',', ':'))
timestamp = str(int(time.time()))
signature = hmac.new(secret.encode(), timestamp.encode() + payload.encode(), hashlib.sha256).hexdigest()
print(timestamp)
print(payload)
print(signature)
PY
)"
if [ -z "$CALLBACK_SIGNATURE" ]; then
    echo "❌ Failed to compute HMAC signature for callback"
    exit 1
fi
CALLBACK_RESPONSE=$(curl --fail --silent --show-error \
    -X POST "http://localhost:3000/api/v1/callbacks/mpesa" \
    -H "Content-Type: application/json" \
    -H "X-Request-Timestamp: $CALLBACK_TS" \
    -H "X-Mpesa-Signature: $CALLBACK_SIGNATURE" \
    -d "$CALLBACK_BODY")
CALLBACK_STATUS=$(echo "$CALLBACK_RESPONSE" | jq -r '.status')
echo "   → Callback processed (status: $CALLBACK_STATUS)"

echo "5️⃣ Verify mint executed"
sleep 2
INTENT_STATUS=$(cast call "$MINT_ESCROW" "getIntentStatus(bytes32)(uint8)" "$INTENT_ID" --rpc-url "$ETH_RPC_URL")
USER_BALANCE=$(cast call "$COUNTRY_TOKEN" "balanceOf(address)(uint256)" "$TEST_USER_ADDRESS" --rpc-url "$ETH_RPC_URL")
USER_BALANCE_HUMAN=$(cast --from-wei "$USER_BALANCE")
case "$INTENT_STATUS" in
    0) STATUS_LABEL="Pending" ;;
    1) STATUS_LABEL="Executed" ;;
    2) STATUS_LABEL="Refunded" ;;
    3) STATUS_LABEL="Failed" ;;
    *) STATUS_LABEL="Unknown" ;;
esac
echo "   → Intent status: $STATUS_LABEL ($INTENT_STATUS)"
echo "   → Country token balance: $USER_BALANCE_HUMAN ($USER_BALANCE wei)"

echo ""
echo "════════════════════════════════════════════════════════"
echo "                    DEMO COMPLETE"
echo "════════════════════════════════════════════════════════"
echo ""
echo "Next steps:"
echo "  - Open Grafana: http://localhost:3001 (admin/admin)"
echo "  - View Prometheus: http://localhost:9090"
echo "  - Check API health: http://localhost:3000/health"
echo "  - View API metrics: http://localhost:3000/metrics"
echo ""
echo "To stop:"
echo "  docker compose down"
echo ""
