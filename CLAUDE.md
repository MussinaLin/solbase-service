# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

X402 Cross-Chain Payment API Backend (Go) - Enables cross-chain payments from multiple source chains (Solana, Base, BSC) to Base using the X402 protocol. Users can pay on their preferred chain using either an email address or wallet address as receiver. Emails are resolved to Base wallets via Privy. Merchants always receive USDC on Base chain.

## Development Commands

```bash
# Run
make run                      # Run the application
make dev                      # Run with hot reload (requires air)
go run ./cmd/api/main.go      # Run directly

# Build
make build                    # Build binary to ./bin/server
go build -o bin/server ./cmd/api

# Database
docker-compose up -d          # Start PostgreSQL
make sqlc                     # Generate Go code from SQL (runs in database/)
cd database && sqlc generate  # Alternative

# Database Migrations (goose)
make migrate-up               # Run all pending migrations
make migrate-down             # Rollback the last migration
make migrate-status           # Show migration status
make migrate-create name=xxx  # Create a new migration

# Test
make test                     # Run unit tests
make test-coverage            # Run tests with coverage report
go test -v ./internal/payment/service/...  # Test specific package

# Code Quality
make fmt                      # Format code
make lint                     # Lint code (requires golangci-lint)
make tidy                     # Tidy go modules

# Tools
make install-tools            # Install air and golangci-lint
```

## Architecture

### Layer Architecture

**API → Service → Repository → Domain**

- **Domain layer** (`internal/payment/payment.go`) - Defines interfaces, models, and constants
- **Service layer** (`internal/payment/service/`) - Implements business logic
- **Repository layer** (`internal/payment/repository/`) - Handles data access with sqlc
- **API layer** (`internal/payment/api/`) - Thin HTTP handlers using chi + httpwrap patterns

### API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/intents` | Create intent (email OR wallet address + amount) |
| POST | `/api/intents/{intent_id}` | Submit X402 proof for existing intent |
| GET | `/api/intents?intent_id={id}` | Get combined status + receipt |
| GET | `/health` | Health check |

### Create Intent Request Options

**Option 1 - Email (resolves to wallet via Privy):**
```json
{ "email": "receiver@example.com", "amount": "10.00", "payer_chain": "solana" }
```

**Option 2 - Direct Wallet Address:**
```json
{ "recipient": "0x742d35Cc...", "amount": "10.00", "payer_chain": "base" }
```

**Supported Payer Chains:** `solana`, `base`, `bsc` (default: `solana`)

### Payment Flow

```
1. POST /intents (email OR recipient + amount + payer_chain)
   → If email: resolve to Base wallet via Privy
   → If recipient: use wallet address directly
   → Create intent in AWAITING_PAYMENT status
   → Return intent_id + wallet address + payer_chain

2. Client completes X402 payment on selected chain (external)

3. POST /intents/{intent_id} (settle_proof)
   → Validate proof matches intent (amount)
   → Update status to PENDING
   → Trigger async processing

4. Async Processing (goroutine):
   PENDING → VERIFICATION_FAILED (invalid proof)
          → SOURCE_SETTLED (proof verified on selected chain)
                ↓
          BASE_SETTLING
                ↓
          BASE_SETTLED (success) / SOURCE_SETTLED (rollback)

5. AWAITING_PAYMENT/PENDING → EXPIRED (10 min timeout)
```

### Project Structure

```
├── cmd/api/main.go                     # Entry point
├── database/
│   ├── db.go                           # Embedded migrations
│   ├── migrations/                     # SQL migration files (goose format)
│   │   ├── 00001_create_payment_intents.sql
│   │   └── 00002_create_email_wallets.sql
│   ├── queries/                        # sqlc query files
│   │   ├── payment_intent.sql
│   │   └── email_wallet.sql
│   └── sqlc.yaml                       # sqlc configuration
├── internal/
│   ├── api/                            # Main API setup and routing
│   │   ├── route.go                    # Central routing with chi
│   │   └── middleware/                 # HTTP middlewares
│   │       ├── logger.go
│   │       ├── error.go
│   │       └── cors.go
│   ├── config/config.go                # Configuration management
│   ├── db/                             # sqlc generated code
│   │   ├── db.go
│   │   ├── models.go
│   │   ├── payment_intent.sql.go
│   │   └── email_wallet.sql.go
│   ├── httpwrap/                       # HTTP response helpers
│   │   ├── httpwrap.go                 # Response/ErrorResponse types
│   │   ├── handler.go                  # Handler wrapper function
│   │   └── bindings.go                 # BindBody, error helpers
│   ├── storage/                        # Database connections
│   │   └── postgres.go                 # pgx connection + migrations
│   ├── payment/                        # Payment domain
│   │   ├── payment.go                  # Domain models, interfaces, constants
│   │   ├── errors.go                   # Domain-specific errors
│   │   ├── service/                    # Business logic layer
│   │   │   ├── service.go              # PaymentIntentService
│   │   │   ├── base_payment.go         # USDC transfers via go-ethereum
│   │   │   ├── x402_verifier.go        # X402 proof verification
│   │   │   └── privy.go                # Privy API for wallet management
│   │   ├── repository/                 # Data access layer
│   │   │   ├── repository.go           # Repository interface
│   │   │   └── sqlc/sqlc.go            # sqlc implementation
│   │   └── api/                        # HTTP handlers
│   │       └── api.go                  # AddRoutes + handlers + DTOs
│   └── health/api/                     # Health check domain
│       └── api.go
├── pkg/utils/address.go                # Utility functions
├── test.html                           # Browser-based API tester (with auto-flow)
├── Makefile                            # Build commands
└── docker-compose.yml                  # PostgreSQL
```

### Key Dependencies

- `go-chi/chi/v5` - HTTP router
- `jackc/pgx/v5` - PostgreSQL driver
- `sqlc` - Type-safe SQL code generation
- `pressly/goose/v3` - Database migrations (embedded)
- `ethereum/go-ethereum` - Base chain integration
- `coinbase/x402/go` - X402 facilitator client

### Database (sqlc + goose)

SQL migrations in `database/migrations/` (goose format with `-- +goose Up/Down` directives), queries in `database/queries/`. Run `make sqlc` or `cd database && sqlc generate` to regenerate Go code. Migrations are embedded and run automatically on startup.

**Tables:**
- `payment_intents` - Payment intent records with status tracking
- `email_wallets` - Email-to-wallet mapping cache

**Status Constants** (in `internal/payment/payment.go`):
- `AWAITING_PAYMENT`, `PENDING`, `VERIFICATION_FAILED`, `SOURCE_SETTLED`, `BASE_SETTLING`, `BASE_SETTLED`, `EXPIRED`

## Key Implementation Details

### USDC Contract Addresses

Hardcoded in `internal/payment/service/base_payment.go`:
- **Base Sepolia**: `0x036CbD53842c5426634e7929541eC2318f3dCF7e`
- **Base Mainnet**: `0x833589fcd6edb6e08f4c7c32d4f71b54bda02913`

### Privy Integration

The `PrivyService` (`internal/payment/service/privy.go`) handles email-to-wallet resolution:
1. Check local database cache for existing mapping
2. If not found, query Privy API for user by email
3. If user doesn't exist, create user with email via Privy import API
4. Create ethereum wallet for user if needed
5. Cache mapping in local database

### Proof Validation

`SubmitProof` validates the X402 proof before storing:
1. Decode proof using `types.DecodePaymentPayloadFromBase64(proof)`
2. Extract amount from `payload.Payload.Authorization.Value`
3. Compare with intent amount (must match)
4. On-chain verification is done asynchronously via X402 facilitator

### X402 Proof Verification

Uses official SDK (`internal/payment/service/x402_verifier.go`):
```go
payload, _ := types.DecodePaymentPayloadFromBase64(proof)
verifyResp, _ := client.Verify(payload, requirements)
```

Amount is extracted from `payload.Payload.Authorization.Value` (string, USDC microdollars with 6 decimals).

## Environment Variables

```env
# Server
PORT=3001

# Database (PostgreSQL only)
DATABASE_URL=postgresql://x402:x402_dev_password@localhost:5432/x402_payments?sslmode=disable

# Solana (as payer chain)
SOLANA_RECEIVER_ADDRESS=Your_Solana_Address
SOLANA_NETWORK=solana-devnet          # solana-devnet | solana-mainnet-beta

# Base Chain (as target and optionally as payer chain)
BASE_NETWORK=base-sepolia             # base-sepolia | base (target chain)
BASE_SOURCE_NETWORK=base-sepolia      # base-sepolia | base (when Base is payer chain)
BASE_PROXY_PRIVATE_KEY=0x...          # 0x + 64 hex chars

# BSC (as payer chain)
BSC_NETWORK=bsc-testnet               # bsc-testnet | bsc

# Privy (for email-to-wallet)
PRIVY_APP_ID=your-privy-app-id
PRIVY_APP_SECRET=your-privy-app-secret

# X402
FACILITATOR_URL=https://x402.org/facilitator
```

The proxy wallet must have ETH for gas and sufficient USDC balance.

## Testing

### Local Testing Website

Open `test.html` in a browser to test the API:
1. Select payer chain (Solana/Base/BSC) → Choose email or wallet address → Enter amount → Create intent
2. Complete X402 payment on selected chain externally
3. Submit proof → **Auto-polling starts automatically** (when auto-flow enabled)
4. Polling stops automatically when terminal state reached (BASE_SETTLED, VERIFICATION_FAILED, EXPIRED)

**Auto-flow features:**
- Toggle auto-flow ON/OFF in Configuration section
- When enabled: auto-starts polling after proof submission
- When enabled: auto-stops polling when terminal state reached
- Shows notification on terminal state

### Unit Tests

```bash
make test
go test -v ./internal/payment/service/...
```
