# X402 Cross-Chain Payment API Backend (Go)

> Go backend API for X402 cross-chain payment protocol (Solana → Base)
>
> **For client-side developers, try [X402 Cross-Chain Payment SDK](https://github.com/agent-tech/solbase-sdk) to integrate this service to your product.**

## Project Overview

Clean, production-ready Go backend that provides REST API endpoints for the X402 cross-chain payment SDK. It enables users to pay on Solana while merchants receive payments on Base chain.

### Features

- **Simplified API** - 3 endpoints for complete payment lifecycle
- **Proof-First Approach** - Submit X402 proof upfront, async verification
- **Official X402 SDK** - Uses `coinbase/x402/go` for type-safe proof verification
- **Base Chain Integration** - USDC transfers using go-ethereum
- **Dual Database Support** - PostgreSQL (production) + SQLite (development)
- **Structured Logging** - Logrus-based logging system
- **Health Checks** - Database and memory monitoring
- **Testing Website** - Built-in HTML tester for API endpoints

## Quick Start

### Prerequisites

- Go >= 1.23
- PostgreSQL (optional, can use SQLite for dev)

### Installation

```bash
# Install dependencies
go mod download

# Copy environment variables
cp .env.example .env
# Edit .env with your configuration

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

### Simplified Endpoints (3 total)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/intents` | Create intent with X402 proof |
| GET | `/intents?intent_id={id}` | Get status + receipt |
| GET | `/health` | Health check |

### POST /intents - Create Payment Intent

Submit X402 settlement proof and merchant address. Returns immediately, processes asynchronously.

**Request:**
```json
{
  "settle_proof": "eyJhbGciOiJIUzI1NiIs...",
  "merchant_recipient": "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb1"
}
```

**Response:**
```json
{
  "intent_id": "550e8400-e29b-41d4-a716-446655440000",
  "merchant_recipient": "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb1",
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
  "amount": "100.00",
  "merchant_recipient": "0x742d35Cc...",
  "payer_wallet": "0xabc123...",
  "created_at": "2024-01-15T10:30:00Z",
  "completed_at": "2024-01-15T10:31:30Z",
  "solana_payment": {
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
POST /intents (with X402 proof)
    ↓
  PENDING ──────────────────────────> EXPIRED (10 min timeout)
    │
    │ (goroutine: verify X402 proof)
    │
    ├──> VERIFICATION_FAILED (invalid proof)
    │
    └──> SOL_SETTLED (proof verified, amount extracted)
              │
              │ (goroutine: execute Base payment)
              │
              └──> BASE_SETTLING
                        │
                        ├──> BASE_SETTLED (success)
                        │
                        └──> SOL_SETTLED (rollback on failure)
```

## Project Structure

```
.
├── cmd/server/main.go              # Application entry point
├── internal/
│   ├── config/config.go            # Configuration management
│   ├── database/db.go              # Database connection (GORM)
│   ├── models/payment_intent.go    # Domain models + status helpers
│   ├── dto/requests.go             # Request/response DTOs
│   ├── services/
│   │   ├── payment_intent.go       # Business logic + async processing
│   │   ├── base_payment.go         # Base USDC transfers
│   │   └── x402_verifier.go        # X402 proof verification
│   ├── handlers/
│   │   ├── payment_intents.go      # HTTP handlers
│   │   └── health.go               # Health check
│   └── middleware/
│       ├── logger.go               # Request logging
│       └── error.go                # Error handling
├── test.html                       # Browser-based API tester
├── Makefile                        # Build commands
├── CLAUDE.md                       # Claude Code instructions
└── docker-compose.yml              # PostgreSQL + Redis
```

## Database Configuration

### SQLite (Development)

```env
DATABASE_URL="file:./dev.db"
```

### PostgreSQL (Production)

```env
DATABASE_URL="postgresql://user:password@localhost:5432/x402_payments?sslmode=disable"
```

## Environment Variables

### Required

```env
PORT=3001
DATABASE_URL="file:./dev.db"
SOLANA_RECEIVER_ADDRESS=Your_Solana_Address
SOLANA_NETWORK=solana-devnet
BASE_NETWORK=base-sepolia
BASE_PROXY_PRIVATE_KEY=0xYourPrivateKey
```

### Optional

```env
FACILITATOR_URL=https://x402.org/facilitator
LOG_LEVEL=debug
CORS_ORIGINS=http://localhost:3000
```

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
make install-tools    # Install air and golangci-lint
```

### Testing Website

Open `test.html` in a browser to test the API:
1. Enter X402 settlement proof and merchant address
2. Click "Create Intent" to submit
3. Click "Start Polling" to watch status updates
4. View combined status + receipt data

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
    github.com/coinbase/x402/go      // X402 SDK
    github.com/ethereum/go-ethereum  // Base chain
    github.com/gin-gonic/gin         // Web framework
    gorm.io/gorm                     // ORM
    github.com/sirupsen/logrus       // Logging
)
```

## License

MIT

## Additional Resources

- [Go Documentation](https://go.dev/doc/)
- [Gin Framework](https://gin-gonic.com/)
- [GORM](https://gorm.io/)
- [go-ethereum](https://geth.ethereum.org/)
- [X402 Go SDK](https://github.com/coinbase/x402/tree/main/go)
- [X402 Protocol](https://github.com/coinbase/x402)

---

**Built for the [X402 Cross-Chain Payment SDK](https://github.com/agent-tech/solbase-sdk)**
