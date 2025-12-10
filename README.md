# X402 Cross-Chain Payment API Backend (Go)

> Go backend API for X402 cross-chain payment protocol (Solana/Base/BSC → Base)
>
> **For client-side developers, try [X402 Cross-Chain Payment SDK](https://github.com/agent-tech/solbase-sdk) to integrate this service to your product.**

## Project Overview

Clean, production-ready Go backend that provides REST API endpoints for the X402 cross-chain payment SDK. It enables users to pay on multiple chains (Solana, Base, BSC) using either email addresses or wallet addresses as receivers. Emails are automatically resolved to Base chain wallets via Privy. All payments are settled on Base chain.

### Features

- **Multi-Chain Support** - Pay on Solana, Base, or BSC → receive on Base
- **Email-to-Wallet Payments** - Send USDC to Base using just an email address
- **Direct Wallet Payments** - Or send directly to a Base wallet address
- **Privy Integration** - Automatic wallet creation and management via Privy API
- **Two-Step Flow** - Create intent first, then submit proof after payment
- **Proof Validation** - Validates X402 proof matches intent before processing
- **On-Chain Verification** - Uses X402 facilitator to verify on-chain transactions
- **Base Chain Integration** - USDC transfers using go-ethereum
- **sqlc + PostgreSQL** - Type-safe SQL queries with goose migrations
- **Testing Website** - Built-in HTML tester for API endpoints

## Quick Start

### Prerequisites

- Go >= 1.23
- PostgreSQL (via Docker recommended)
- sqlc (`brew install sqlc`)
- goose (`go install github.com/pressly/goose/v3/cmd/goose@latest`)
- Privy account (for email-to-wallet feature)

### Installation

```bash
# Start PostgreSQL
docker-compose up -d

# Install dependencies
go mod download

# Copy environment variables
cp .env.example .env
# Edit .env with your configuration

# Generate sqlc code (if needed)
sqlc generate

# Run the application
make run
# Or directly:
go run ./cmd/server/main.go
```

The API will be available at `http://localhost:3001`

## API Endpoints

### Base URL

```
http://localhost:3001/api
```

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/intents` | Create intent (email OR wallet + amount) |
| POST | `/intents/{intent_id}` | Submit X402 proof |
| GET | `/intents?intent_id={id}` | Get status + receipt |
| GET | `/health` | Health check |

### POST /intents - Create Payment Intent

Create a payment intent with receiver's email address OR wallet address. Returns wallet address for X402 payment.

**Request Option 1 - Email:**
```json
{
  "email": "receiver@example.com",
  "amount": "10.00",
  "payer_chain": "solana"
}
```

**Request Option 2 - Wallet Address:**
```json
{
  "recipient": "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb1",
  "amount": "10.00",
  "payer_chain": "base"
}
```

**Supported Payer Chains:** `solana`, `base`, `bsc` (default: `solana`)

**Response:**
```json
{
  "intent_id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "receiver@example.com",
  "merchant_recipient": "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb1",
  "amount": "10.00",
  "payer_chain": "solana",
  "status": "AWAITING_PAYMENT",
  "created_at": "2024-01-15T10:30:00Z",
  "expires_at": "2024-01-15T10:40:00Z"
}
```

Note: `email` field is only included in response when email was provided in request.

### POST /intents/{intent_id} - Submit Proof

Submit X402 settlement proof for an existing payment intent.

**Request:**
```json
{
  "settle_proof": "eyJhbGciOiJIUzI1NiIs..."
}
```

**Response:**
```json
{
  "intent_id": "550e8400-e29b-41d4-a716-446655440000",
  "merchant_recipient": "0x742d35Cc...",
  "status": "PENDING",
  "created_at": "2024-01-15T10:30:00Z",
  "expires_at": "2024-01-15T10:40:00Z"
}
```

### GET /intents - Get Status + Receipt

Poll this endpoint to track payment progress.

**Response (completed):**
```json
{
  "intent_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "BASE_SETTLED",
  "amount": "10.00",
  "payer_chain": "solana",
  "merchant_recipient": "0x742d35Cc...",
  "receiver_email": "receiver@example.com",
  "payer_wallet": "0xabc123...",
  "created_at": "2024-01-15T10:30:00Z",
  "completed_at": "2024-01-15T10:31:30Z",
  "source_payment": {
    "chain": "solana",
    "tx_hash": "5abc...",
    "settle_proof": "eyJ...",
    "settled_at": "2024-01-15T10:30:30Z",
    "explorer_url": "https://solscan.io/tx/5abc..."
  },
  "base_payment": {
    "tx_hash": "0xdef...",
    "settle_proof": "base_settlement_0xdef...",
    "settled_at": "2024-01-15T10:31:30Z",
    "explorer_url": "https://basescan.org/tx/0xdef..."
  }
}
```

## Payment Flow

```
1. POST /intents (email OR recipient + amount + payer_chain)
   ↓
   If email: resolve → Base wallet via Privy
   If recipient: use wallet directly
   ↓
   AWAITING_PAYMENT ────────────────────> EXPIRED (10 min timeout)

2. Client completes X402 payment on selected chain (Solana/Base/BSC)

3. POST /intents/{intent_id} (settle_proof)
   ↓
   Validate proof matches intent
   ↓
   PENDING
   │
   │ (goroutine: verify X402 proof on selected chain)
   │
   ├──> VERIFICATION_FAILED (invalid proof)
   │
   └──> SOURCE_SETTLED (proof verified)
              │
              │ (goroutine: execute Base payment)
              │
              └──> BASE_SETTLING
                        │
                        ├──> BASE_SETTLED (success)
                        │
                        └──> SOURCE_SETTLED (rollback on failure)
```

## Project Structure

```
.
├── cmd/server/main.go              # Application entry point
├── db/
│   ├── db.go                       # Embedded migrations (goose)
│   ├── migrations/                 # SQL migration files (goose format)
│   └── query/                      # sqlc query definitions
├── sqlc.yaml                       # sqlc configuration
├── internal/
│   ├── config/config.go            # Configuration management
│   ├── database/db.go              # pgx connection + migrations
│   ├── db/                         # sqlc generated code
│   ├── dto/requests.go             # Request/response DTOs
│   ├── services/
│   │   ├── payment_intent.go       # Business logic + async processing
│   │   ├── base_payment.go         # Base USDC transfers
│   │   ├── x402_verifier.go        # X402 proof verification
│   │   └── privy.go                # Privy API integration
│   ├── handlers/
│   │   ├── payment_intents.go      # HTTP handlers
│   │   └── health.go               # Health check
│   └── middleware/
│       ├── logger.go               # Request logging
│       └── error.go                # Error handling
├── test.html                       # Browser-based API tester
├── Makefile                        # Build commands
├── CLAUDE.md                       # Claude Code instructions
└── docker-compose.yml              # PostgreSQL
```

## Database Configuration

### PostgreSQL (Required)

```env
DATABASE_URL=postgresql://x402:x402_dev_password@localhost:5432/x402_payments?sslmode=disable
```

Start PostgreSQL with Docker:
```bash
docker-compose up -d
```

Migrations run automatically on startup.

## Environment Variables

### Required

```env
PORT=3001
DATABASE_URL=postgresql://x402:x402_dev_password@localhost:5432/x402_payments?sslmode=disable
SOLANA_RECEIVER_ADDRESS=Your_Solana_Address
SOLANA_NETWORK=solana-devnet
BASE_NETWORK=base-sepolia
BASE_SOURCE_NETWORK=base-sepolia
BSC_NETWORK=bsc-testnet
BASE_PROXY_PRIVATE_KEY=0xYourPrivateKey
PRIVY_APP_ID=your-privy-app-id
PRIVY_APP_SECRET=your-privy-app-secret
```

### Optional

```env
FACILITATOR_URL=https://x402.org/facilitator
LOG_LEVEL=debug
CORS_ORIGINS=http://localhost:3000
```

### Network Reference

| Chain | Testnet | Mainnet |
|-------|---------|---------|
| Solana | solana-devnet | solana-mainnet-beta |
| Base | base-sepolia | base |
| BSC | bsc-testnet | bsc |

## Development

### Available Commands

```bash
make run              # Run the application
make dev              # Run with hot reload (requires air)
make build            # Build binary to ./bin/server
make test             # Run unit tests
make test-coverage    # Run tests with coverage report
make fmt              # Format code
make lint             # Lint code (requires golangci-lint)
make tidy             # Tidy go modules
make install-tools    # Install air, golangci-lint, and goose
sqlc generate         # Regenerate Go code from SQL

# Database migrations (goose)
make migrate-up       # Run all pending migrations
make migrate-down     # Rollback the last migration
make migrate-status   # Show migration status
make migrate-create name=xxx  # Create a new migration
```

### Testing Website

Open `test.html` in a browser to test the API:
1. Select payer chain (Solana/Base/BSC)
2. Choose email or wallet address
3. Enter amount and click "Create Intent"
4. Complete X402 payment on selected chain (external)
5. Submit the X402 proof
6. Click "Start Polling" to watch status updates until BASE_SETTLED

## Deployment

### Build for Production

```bash
make build
./bin/server
```

### Docker

```bash
docker build -t x402-api-backend .
docker run -p 3001:3001 --env-file .env x402-api-backend
```

## Monitoring

### Health Check

```bash
curl http://localhost:3001/health
```

```json
{
  "status": "ok",
  "info": {
    "database": { "status": "up" },
    "memory": { "status": "up", "alloc_mb": 12, "sys_mb": 24 }
  }
}
```

## Key Dependencies

```go
require (
    github.com/coinbase/x402/go       // X402 SDK
    github.com/ethereum/go-ethereum   // Base chain
    github.com/gin-gonic/gin          // Web framework
    github.com/jackc/pgx/v5           // PostgreSQL driver
    github.com/pressly/goose/v3       // Database migrations
    github.com/sirupsen/logrus        // Logging
)
```

## License

MIT

## Additional Resources

- [Go Documentation](https://go.dev/doc/)
- [Gin Framework](https://gin-gonic.com/)
- [sqlc](https://sqlc.dev/)
- [go-ethereum](https://geth.ethereum.org/)
- [X402 Go SDK](https://github.com/coinbase/x402/tree/main/go)
- [X402 Protocol](https://github.com/coinbase/x402)
- [Privy Documentation](https://docs.privy.io/)

---

**Built for the [X402 Cross-Chain Payment SDK](https://github.com/agent-tech/solbase-sdk)**
