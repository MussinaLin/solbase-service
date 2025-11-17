.PHONY: help build run test clean docker-build docker-run dev lint fmt

help: ## Display this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the application
	@echo "Building application..."
	go build -o bin/server ./cmd/server

run: ## Run the application
	@echo "Running application..."
	go run ./cmd/server/main.go

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
