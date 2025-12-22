.PHONY: help run build test clean migrate-up migrate-down docker-up docker-down

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $1, $2}'

run: ## Run the application
	go run cmd/api/main.go

build: ## Build the application
	go build -o bin/church-api cmd/api/main.go

test: ## Run tests
	go test -v ./...

clean: ## Clean build artifacts
	rm -rf bin/

deps: ## Download dependencies
	go mod download
	go mod tidy

migrate-up: ## Run database migrations
	psql -U postgres -d church_db -f internal/database/migrations/001_initial_schema.sql

migrate-down: ## Rollback database migrations
	psql -U postgres -d church_db -c "DROP TABLE IF EXISTS testimonials CASCADE;"

docker-build: ## Build Docker image
	docker build -t church-api:latest .

docker-up: ## Start Docker containers
	docker-compose up -d

docker-down: ## Stop Docker containers
	docker-compose down

lint: ## Run linter
	golangci-lint run

dev: ## Run with hot reload (requires air)
	air