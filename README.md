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
- **Testing Website** - Built-in HTML tester with auto-flow for API endpoints

### Security & Reliability

- **Input Validation (Poka-Yoke)** - Email, wallet address, and amount validation
- **Race Condition Prevention** - Optimistic locking prevents double-spend attacks
- **Error Sanitization** - Generic error messages to clients, detailed logs server-side
- **Graceful Shutdown** - Goroutine lifecycle management with WaitGroup
- **Idempotent Resource Cleanup** - Safe concurrent and repeated close operations
- **Request Body Limits** - 1MB limit prevents memory exhaustion attacks

## Architecture

**Layer Architecture: API → Service → Repository → Domain**

- **Domain layer** - Defines interfaces, models, and constants
- **Service layer** - Implements business logic
- **Repository layer** - Handles data access with sqlc
- **API layer** - Thin HTTP handlers using chi + httpwrap patterns

## Quick Start

### Prerequisites

- Go >= 1.24
- Docker & Docker Compose (for PostgreSQL)
- sqlc (`brew install sqlc`)
- goose (`go install github.com/pressly/goose/v3/cmd/goose@latest`)
- Privy account (for email-to-wallet feature) - [Sign up at privy.io](https://privy.io)

### Installation

1. **Clone and install dependencies**
   ```bash
   git clone <repo-url>
   cd solbase-service
   go mod download
   ```

2. **Start PostgreSQL**
   ```bash
   docker-compose up -d
   ```

3. **Configure environment**
   ```bash
   cp .env.example .env
   # Edit .env with your configuration (see Environment Variables section)
   ```

4. **Run the application**
   ```bash
   make run
   # Or: go run ./cmd/api/main.go
   ```
   > **Note:** Database migrations run automatically on startup. For manual control, use `make migrate-up` / `make migrate-down`.

5. **Verify installation**
   ```bash
   curl http://localhost:3001/health
   # Expected: {"status":"ok","info":{"database":{"status":"up"},...}}
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
  "source_recipient": "Your_Solana_Address",
  "amount": "10.00",
  "payer_chain": "solana",
  "status": "AWAITING_PAYMENT",
  "created_at": "2024-01-15T10:30:00Z",
  "expires_at": "2024-01-15T10:40:00Z"
}
```

Note: `email` field is only included when email was provided in request. `source_recipient` is the payment receiver address on the payer chain (Solana/BSC) - only included for non-Base payer chains.

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
   │ (goroutine: verify X402 proof with facilitator)
   │
   ├──> VERIFICATION_FAILED (invalid proof)
   │
   │ (goroutine: settle payment on source chain via X402 facilitator)
   │
   ├──> VERIFICATION_FAILED (settlement failed)
   │
   └──> SOURCE_SETTLED (proof verified + settled on source chain, txHash stored)
            │
            │ (goroutine: execute Base payment)
            │
            └──> BASE_SETTLING
                      │
                      ├──> BASE_SETTLED (success)
                      │
                      └──> SOURCE_SETTLED (rollback on failure)
```

**Note:** The settlement step actually executes the payment on the source chain via the X402 facilitator.
Without settlement, the proof is only verified but funds are not transferred.

## Project Structure

```
.
├── cmd/api/main.go                     # Application entry point
├── database/
│   ├── db.go                           # Embedded migrations
│   ├── migrations/                     # SQL migration files (goose format)
│   ├── queries/                        # sqlc query definitions
│   └── sqlc.yaml                       # sqlc configuration
├── internal/
│   ├── api/                            # Main API setup and routing
│   │   ├── route.go                    # Central routing with chi
│   │   └── middleware/                 # HTTP middlewares
│   ├── config/config.go                # Configuration management
│   ├── db/                             # sqlc generated code
│   ├── httpwrap/                       # HTTP response helpers
│   ├── storage/                        # Database connections
│   ├── payment/                        # Payment domain
│   │   ├── payment.go                  # Domain models, interfaces
│   │   ├── errors.go                   # Domain errors (validation, concurrency, etc.)
│   │   ├── service/                    # Business logic layer
│   │   │   ├── service.go              # PaymentIntentService
│   │   │   ├── base_payment.go         # Base USDC transfers
│   │   │   ├── x402_verifier.go        # X402 proof verification
│   │   │   └── privy.go                # Privy API integration
│   │   ├── repository/                 # Data access layer
│   │   └── api/                        # HTTP handlers + DTOs
│   └── health/api/                     # Health check
├── test.html                           # Browser-based API tester (with auto-flow)
├── Makefile                            # Build commands
├── CLAUDE.md                           # Claude Code instructions
└── docker-compose.yml                  # PostgreSQL
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
# Server
PORT=3001                              # HTTP server port

# Database
DATABASE_URL=postgresql://x402:x402_dev_password@localhost:5432/x402_payments?sslmode=disable

# Solana (when used as payer chain)
SOLANA_RECEIVER_ADDRESS=Your_Solana_Address    # X402 payment receiver on Solana
SOLANA_NETWORK=solana-devnet                   # solana-devnet | solana-mainnet-beta

# Base Chain (settlement destination)
BASE_NETWORK=base-sepolia                      # base-sepolia | base

# Base Chain (when used as payer chain)
BASE_SOURCE_NETWORK=base-sepolia               # base-sepolia | base

# BSC (when used as payer chain)
BSC_RECEIVER_ADDRESS=Your_BSC_Address          # X402 payment receiver on BSC
BSC_NETWORK=bsc-testnet                        # bsc-testnet | bsc

# Proxy Wallet (CRITICAL - must have ETH for gas + USDC for transfers)
BASE_PROXY_PRIVATE_KEY=0xYourPrivateKey        # Private key for USDC transfers on Base

# Privy (email-to-wallet resolution)
PRIVY_APP_ID=your-privy-app-id                 # From Privy dashboard
PRIVY_APP_SECRET=your-privy-app-secret         # From Privy dashboard
```

### Optional

```env
FACILITATOR_URL=https://x402.org/facilitator   # X402 proof verification endpoint
LOG_LEVEL=debug                                # error | warn | info | debug
CORS_ORIGINS=http://localhost:3000             # Comma-separated allowed origins
```

### Network Reference

| Chain | Testnet | Mainnet |
|-------|---------|---------|
| Solana | solana-devnet | solana-mainnet-beta |
| Base | base-sepolia | base |
| BSC | bsc-testnet | bsc |

## Wallet Setup

### Proxy Wallet Requirements

The `BASE_PROXY_PRIVATE_KEY` wallet executes USDC transfers on Base chain. It must have:

- **ETH** - For gas fees (~0.01 ETH recommended for testnet)
- **USDC** - Balance to cover payment amounts

### Getting Testnet Funds

**Base Sepolia:**
- ETH Faucet: https://www.alchemy.com/faucets/base-sepolia
- USDC Contract: `0x036CbD53842c5426634e7929541eC2318f3dCF7e`
  - Get testnet USDC from [Coinbase Faucet](https://faucet.circle.com/) or bridge from other testnets

**Solana Devnet:**
- SOL Faucet: `solana airdrop 2` (with [Solana CLI](https://docs.solana.com/cli/install-solana-cli-tools))
- USDC: Devnet USDC requires minting from test programs

### Setting Up Privy

1. Create account at [privy.io](https://privy.io)
2. Create a new app in the Privy dashboard
3. Copy **App ID** and **App Secret** to your `.env` file
4. Enable "Email" as a login method in the dashboard

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
make sqlc             # Regenerate Go code from SQL

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
6. **Auto-polling starts automatically** (when auto-flow enabled)
7. Polling stops when terminal state reached (BASE_SETTLED, VERIFICATION_FAILED, EXPIRED)

**Auto-flow features:**
- Toggle auto-flow ON/OFF in Configuration section
- Auto-starts polling after proof submission
- Auto-stops polling when terminal state reached
- Shows notification on terminal state

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

## Security

### Input Validation

All inputs are validated at the service layer before processing:

| Field | Validation | Error |
|-------|------------|-------|
| Email | RFC 5322 compliant (`net/mail.ParseAddress`) | `invalid email format` |
| Recipient | Ethereum address format (`^0x[a-fA-F0-9]{40}$`) | `invalid recipient address format` |
| Amount | Positive, max 6 decimals, range 0.01-1,000,000 | `invalid amount` |

### Concurrency Safety

- **Optimistic Locking**: Proof submission uses `UPDATE ... WHERE status = expected_status` to prevent double-spend attacks
- **Goroutine Lifecycle**: Async operations tracked with `sync.WaitGroup` for graceful shutdown
- **Idempotent Close**: Resource cleanup methods use `sync.Once` to prevent double-close panics

### Error Handling

- **Sanitized Responses**: Internal errors return generic messages to clients (e.g., "internal server error")
- **Detailed Logging**: Full error details logged server-side with `logrus`
- **Domain Errors**: Typed errors in `internal/payment/errors.go` for consistent handling

### Request Limits

- **Body Size**: 1MB maximum request body size (prevents memory exhaustion)
- **Timeout**: HTTP client timeout of 30 seconds for external API calls

## Monitoring

### Health Check

```bash
curl http://localhost:3001/health
```

```json
{
  "status": "healthy",
  "info": {
    "database": { "status": "healthy" },
    "memory": { "alloc_mb": 12, "total_alloc_mb": 24, "sys_mb": 36, "num_gc": 5 },
    "goroutines": 10
  }
}
```

## Key Dependencies

```go
require (
    github.com/coinbase/x402/go       // X402 SDK
    github.com/ethereum/go-ethereum   // Base chain
    github.com/go-chi/chi/v5          // Web framework
    github.com/jackc/pgx/v5           // PostgreSQL driver
    github.com/pressly/goose/v3       // Database migrations
    github.com/sirupsen/logrus        // Logging
)
```

## Troubleshooting

### Common Issues

**Database connection failed**
```
Error: failed to connect to database
```
- Ensure PostgreSQL is running: `docker-compose ps`
- Check `DATABASE_URL` in `.env` matches credentials in `docker-compose.yml`
- Verify port 5432 is not in use: `lsof -i :5432`

**Migration errors**
```
Error: migration failed
```
- Check database is accessible: `docker-compose logs postgres`
- Reset migrations if needed: `make migrate-reset`
- Verify `database/migrations/` directory contains SQL files

**"insufficient funds" on Base payment**
- Proxy wallet needs both ETH (for gas) and USDC (for transfers)
- Check wallet balance: [Base Sepolia Explorer](https://sepolia.basescan.org)
- See [Wallet Setup](#wallet-setup) section for faucet links

**Privy API errors**
- Verify `PRIVY_APP_ID` and `PRIVY_APP_SECRET` are correct
- Check Privy dashboard for API status and rate limits
- Ensure "Email" login method is enabled in Privy dashboard

**X402 verification failed**
- Ensure `FACILITATOR_URL` is accessible (default: https://x402.org/facilitator)
- Verify payment was made to the correct address on the correct network
- Check that the proof matches the intent amount

**Validation errors**
- `invalid email format` - Email must be RFC 5322 compliant (e.g., `user@example.com`)
- `invalid recipient address format` - Must be valid Ethereum address (`0x` + 40 hex chars)
- `invalid amount` - Must be positive, max 6 decimal places, range 0.01-1,000,000

**Concurrent proof submission error**
- `concurrent update detected` - Another request already submitted proof for this intent
- This is expected behavior when multiple clients submit proofs simultaneously

**CORS errors in browser**
- Add your frontend URL to `CORS_ORIGINS` in `.env`
- Example: `CORS_ORIGINS=http://localhost:3000,http://localhost:5173`

## License

MIT

## Additional Resources

- [Go Documentation](https://go.dev/doc/)
- [Chi Router](https://go-chi.io/)
- [sqlc](https://sqlc.dev/)
- [go-ethereum](https://geth.ethereum.org/)
- [X402 Go SDK](https://github.com/coinbase/x402/tree/main/go)
- [X402 Protocol](https://github.com/coinbase/x402)
- [Privy Documentation](https://docs.privy.io/)

---

**Built for the [X402 Cross-Chain Payment SDK](https://github.com/agent-tech/solbase-sdk)**
