.PHONY: help build run test clean docker-build docker-run dev lint fmt migrate-up migrate-down migrate-status migrate-create migrate-reset

help: ## Display this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the application
	@echo "Building application..."
	go build -o bin/server ./cmd/api

run: ## Run the application
	@echo "Running application..."
	go run ./cmd/api/main.go

dev: ## Run with hot reload (requires air)
	@echo "Running with hot reload..."
	air

test: ## Run tests
	@echo "Running tests..."
	go test -v ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html

fmt: ## Format code
	@echo "Formatting code..."
	go fmt ./...

lint: ## Lint code
	@echo "Linting code..."
	golangci-lint run

tidy: ## Tidy go modules
	@echo "Tidying modules..."
	go mod tidy

download: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t x402-api-backend:latest .

docker-run: ## Run Docker container
	@echo "Running Docker container..."
	docker run -p 3001:3001 --env-file .env x402-api-backend:latest

install-tools: ## Install development tools
	@echo "Installing development tools..."
	go install github.com/cosmtrek/air@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest

# Database migrations (using goose)
migrate-up: ## Run all pending migrations
	@echo "Running migrations..."
	goose -dir database/migrations postgres "$(DATABASE_URL)" up

migrate-down: ## Rollback the last migration
	@echo "Rolling back last migration..."
	goose -dir database/migrations postgres "$(DATABASE_URL)" down

migrate-status: ## Show migration status
	@echo "Migration status..."
	goose -dir database/migrations postgres "$(DATABASE_URL)" status

migrate-create: ## Create a new migration (usage: make migrate-create name=migration_name)
	@echo "Creating migration $(name)..."
	goose -dir database/migrations create $(name) sql

migrate-reset: ## Reset all migrations (down then up)
	@echo "Resetting all migrations..."
	goose -dir database/migrations postgres "$(DATABASE_URL)" reset
	goose -dir database/migrations postgres "$(DATABASE_URL)" up

sqlc: ## Generate sqlc code
	@echo "Generating sqlc code..."
	cd database && sqlc generate
