# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

X402 Cross-Chain Payment API Backend (Go) - Enables cross-chain payments from Solana to Base using the X402 protocol. Users pay on Solana, merchants receive USDC on Base chain via a proxy wallet.

## Development Commands

```bash
cd golang-version

# Run
make run                      # Run the application
make dev                      # Run with hot reload (requires air)

# Build
make build                    # Build binary to ./bin/server
go build -o bin/server ./cmd/server

# Test
make test                     # Run unit tests
make test-coverage            # Run tests with coverage report
go test -v ./internal/services/...  # Test specific package

# Code Quality
make fmt                      # Format code
make lint                     # Lint code (requires golangci-lint)
make tidy                     # Tidy go modules

# Tools
make install-tools            # Install air and golangci-lint
```

## Architecture

### Simplified API (3 Endpoints)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/intents` | Create intent with X402 proof (async processing) |
| GET | `/api/intents?intent_id={id}` | Get combined status + receipt |
| GET | `/health` | Health check |

### Payment Flow (Proof-First Approach)

```
POST /intents (settle_proof + merchant_recipient)
    ↓
PENDING ──────────────────────────────> EXPIRED (10 min)
    │
    │ (goroutine: verify X402 proof)
    │
    ├──> VERIFICATION_FAILED (invalid proof)
    │
    └──> SOL_SETTLED (proof verified)
              │
              │ (goroutine: trigger Base payment)
              │
              └──> BASE_SETTLING
                        │
                        ├──> BASE_SETTLED (success)
                        │
                        └──> SOL_SETTLED (rollback on failure)
```

### Core Services (`golang-version/internal/services/`)

- **PaymentIntentService** (`payment_intent.go`) - Business logic, async processing via goroutines with 5-minute context timeout
- **BasePaymentService** (`base_payment.go`) - USDC transfers via go-ethereum
- **X402Verifier** (`x402_verifier.go`) - Proof verification using official `coinbase/x402/go` SDK

### Key Dependencies

- `gin-gonic/gin` - HTTP router
- `gorm.io/gorm` - ORM (SQLite/PostgreSQL)
- `ethereum/go-ethereum` - Base chain integration
- `coinbase/x402/go` - X402 facilitator client

### Database Model

`internal/models/payment_intent.go` - GORM model with:
- Status constants: `PENDING`, `VERIFICATION_FAILED`, `SOL_SETTLED`, `BASE_SETTLING`, `BASE_SETTLED`, `EXPIRED`
- Helper methods: `CanTriggerBasePayment()`, `IsExpired()`
- ErrorMessage field for storing verification/processing errors

## Key Implementation Details

### USDC Contract Addresses

Hardcoded in `internal/services/base_payment.go`:
- **Base Sepolia**: `0x036CbD53842c5426634e7929541eC2318f3dCF7e`
- **Base Mainnet**: `0x833589fcd6edb6e08f4c7c32d4f71b54bda02913`

### Async Processing

POST /intents triggers a goroutine with 5-minute context timeout (`internal/services/payment_intent.go:69-74`):
1. Verify X402 proof → extract amount, payer from payload
2. Update to SOL_SETTLED
3. Execute Base USDC transfer
4. Update to BASE_SETTLED (or rollback to SOL_SETTLED on failure)

### X402 Proof Verification

Uses official SDK (`internal/services/x402_verifier.go`):
```go
payload, _ := types.DecodePaymentPayloadFromBase64(proof)
verifyResp, _ := client.Verify(payload, requirements)
```

Amount is extracted from `payload.Payload.Authorization.Value` (string, USDC microdollars).

## Environment Variables

```env
DATABASE_URL                 # file:./dev.db (SQLite) or postgresql://...
SOLANA_RECEIVER_ADDRESS      # Solana wallet for receiving payments
BASE_PROXY_PRIVATE_KEY       # 0x + 64 hex chars
SOLANA_NETWORK              # solana-devnet | solana-mainnet-beta
BASE_NETWORK                # base-sepolia | base
PORT                        # Default: 3001
FACILITATOR_URL             # Default: https://x402.org/facilitator
```

The proxy wallet must have ETH for gas and sufficient USDC balance.

## Testing

### Local Testing Website

Open `golang-version/test.html` in a browser to test the API:
- Create intent with X402 proof
- Poll for status updates
- Health check

### Unit Tests

```bash
make test
go test -v ./internal/services/...
```

- Mock database with GORM's in-memory SQLite
- Test state machine transitions
- Test async processing flow
