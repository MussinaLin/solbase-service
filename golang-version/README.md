# X402 Cross-Chain Payment API Backend (Go)

> Go 1.23+ backend API for X402 cross-chain payment protocol (Solana → Base)
>
> This is a Go rewrite of the TypeScript/NestJS backend with 100% feature parity.

## Project Overview

Clean, production-ready Go backend that provides REST API endpoints for the X402 cross-chain payment SDK. It enables users to pay on Solana while merchants receive payments on Base chain.

### Features

- RESTful API with 5 endpoints - Complete payment intent lifecycle
- Official X402 Go SDK integration - Type-safe proof verification
- Base Chain Integration - USDC transfers using go-ethereum
- Dual Database Support - PostgreSQL (production) + SQLite (development)
- Structured Logging - Logrus-based logging system
- Health Checks - Database and memory monitoring
- CORS Support - Configurable cross-origin requests
- Docker Ready - Docker Compose for PostgreSQL + Redis

## Quick Start

### Prerequisites

- Go >= 1.23
- PostgreSQL (optional, can use SQLite for dev)
- Make (optional, for using Makefile commands)

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

### Using Docker

```bash
# Start PostgreSQL and Redis
docker-compose up -d

# Update DATABASE_URL in .env to:
# DATABASE_URL="postgresql://x402:x402_password@localhost:5432/x402_payments?sslmode=disable"

# Build and run the app
make docker-build
make docker-run
```

## API Endpoints

### Base URL

```
http://localhost:3001/api
```

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/intents` | Create payment intent |
| GET | `/intents?intent_id={id}` | Get payment intent status |
| POST | `/intents/:id/solana-proof` | Submit Solana payment proof |
| POST | `/intents/:id/trigger-base-payment` | Trigger Base chain payment |
| GET | `/intents/:id/receipt` | Get payment receipt |
| GET | `/health` | Health check endpoint |

## Project Structure

```
.
├── cmd/
│   └── server/
│       └── main.go                 # Application entry point
├── internal/
│   ├── config/
│   │   └── config.go              # Configuration management
│   ├── database/
│   │   └── db.go                  # Database connection (GORM)
│   ├── models/
│   │   └── payment_intent.go      # Domain models
│   ├── dto/
│   │   └── requests.go            # Request/response DTOs
│   ├── services/
│   │   ├── payment_intent.go      # Business logic
│   │   ├── base_payment.go        # Base USDC transfers
│   │   └── x402_verifier.go       # X402 proof verification
│   ├── handlers/
│   │   ├── payment_intents.go     # HTTP handlers
│   │   └── health.go              # Health check
│   └── middleware/
│       ├── logger.go              # Request logging
│       └── error.go               # Error handling
├── pkg/
│   └── utils/
│       └── address.go             # Utility functions
├── .env.example                   # Environment template
├── Dockerfile                     # Docker build
├── Makefile                       # Build commands
└── docker-compose.yml             # PostgreSQL + Redis
```

## Database Configuration

### SQLite (Development)

```env
DATABASE_URL="file:./dev.db"
```

No additional setup required. Database file will be created automatically.

### PostgreSQL (Production)

```env
DATABASE_URL="postgresql://user:password@localhost:5432/x402_payments?sslmode=disable"
```

Start PostgreSQL with Docker:

```bash
docker-compose up -d postgres
```

## Environment Variables

### Required

```env
# Server
PORT=3001
NODE_ENV=development

# Database
DATABASE_URL="file:./dev.db"

# Solana
SOLANA_RECEIVER_ADDRESS=Your_Solana_Address
SOLANA_NETWORK=solana-devnet

# Base Chain
BASE_NETWORK=base-sepolia
BASE_PROXY_PRIVATE_KEY=0xYourPrivateKey
```

### Optional

```env
# X402
FACILITATOR_URL=https://x402.org/facilitator

# Logging
LOG_LEVEL=debug

# CORS
CORS_ORIGINS=http://localhost:3000
```

## Development

### Available Commands (Makefile)

```bash
# Development
make run              # Run the application
make dev              # Run with hot reload (requires air)

# Building
make build            # Build binary to ./bin/server

# Testing
make test             # Run unit tests
make test-coverage    # Run tests with coverage report

# Code Quality
make fmt              # Format code
make lint             # Lint code (requires golangci-lint)
make tidy             # Tidy go modules

# Docker
make docker-build     # Build Docker image
make docker-run       # Run in Docker container

# Tools
make install-tools    # Install development tools (air, golangci-lint)
```

### Manual Commands

```bash
# Run
go run ./cmd/server/main.go

# Build
go build -o bin/server ./cmd/server

# Test
go test -v ./...

# Format
go fmt ./...
```

## Key Implementation Details

### X402 SDK Integration

Uses official x402 Go SDK (`github.com/coinbase/x402/go`):

```go
import (
    "github.com/coinbase/x402/go/pkg/facilitatorclient"
    "github.com/coinbase/x402/go/pkg/types"
)

// Create facilitator client
config := &types.FacilitatorConfig{
    URL: "https://x402.org/facilitator",
}
client := facilitatorclient.NewFacilitatorClient(config)

// Verify proof
payload, _ := types.DecodePaymentPayloadFromBase64(proof)
verifyResp, _ := client.Verify(payload, requirements)
```

### Base USDC Transfers

Uses go-ethereum for ERC20 transfers:

```go
import "github.com/ethereum/go-ethereum"

// USDC contract addresses
// Base Sepolia: 0x036CbD53842c5426634e7929541eC2318f3dCF7e
// Base Mainnet: 0x833589fcd6edb6e08f4c7c32d4f71b54bda02913
```

### State Machine

Payment flow: `PENDING` → `SOL_SETTLED` → `BASE_SETTLING` → `BASE_SETTLED`

Automatic rollback to `SOL_SETTLED` if Base payment fails.

### Async Base Payment

After Solana proof verification, Base payment is triggered asynchronously:

```go
go func() {
    if err := s.triggerBasePaymentAsync(intentID); err != nil {
        log.WithError(err).Error("Failed to trigger Base payment")
    }
}()
```

## Dependencies

```go
require (
    github.com/coinbase/x402/go v0.0.0        // X402 SDK
    github.com/ethereum/go-ethereum v1.14.12  // Base chain
    github.com/gin-gonic/gin v1.10.0          // Web framework
    github.com/sirupsen/logrus v1.9.3         // Logging
    gorm.io/gorm v1.25.12                     // ORM
    gorm.io/driver/postgres v1.5.9            // PostgreSQL
    gorm.io/driver/sqlite v1.5.6              // SQLite
    github.com/google/uuid v1.6.0             // UUID
    github.com/go-playground/validator/v10    // Validation
)
```

## Deployment

### Build for Production

```bash
# Build binary
make build

# Set environment
export NODE_ENV=production

# Run
./bin/server
```

### Docker Deployment

```bash
# Build image
docker build -t x402-api-backend .

# Run container
docker run -p 3001:3001 --env-file .env x402-api-backend
```

## Monitoring

### Health Check

```bash
curl http://localhost:3001/health
```

Response:

```json
{
  "status": "ok",
  "info": {
    "database": { "status": "up" },
    "memory": {
      "status": "up",
      "alloc_mb": 12,
      "sys_mb": 24
    }
  }
}
```

### Logs

Logs are output to stdout in text format (development) or JSON format (production).

## Troubleshooting

### Database Connection Issues

```bash
# Check DATABASE_URL
echo $DATABASE_URL

# Test connection (for PostgreSQL)
psql $DATABASE_URL
```

### Private Key Issues

```bash
# Ensure it starts with 0x
# Must be 66 characters (0x + 64 hex chars)
echo $BASE_PROXY_PRIVATE_KEY | wc -c  # Should output 67 (66 + newline)
```

### CORS Errors

Update `CORS_ORIGINS` in `.env`:

```env
CORS_ORIGINS=http://localhost:3000,https://your-frontend.com
```

## Performance Benefits vs TypeScript

- **Faster execution**: Compiled binary, no JIT overhead
- **Lower memory**: ~20-50MB vs 150-300MB for Node.js
- **Better concurrency**: Goroutines vs event loop
- **Faster startup**: ~100ms vs 2-3s for NestJS
- **Single binary deployment**: No node_modules

## Testing

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Test specific package
go test -v ./internal/services/...
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
