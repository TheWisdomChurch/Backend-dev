.PHONY: docker-build docker-up docker-down docker-logs

docker-build: ## Build Docker image
	docker build -t wisdom-house-backend .

docker-up: ## Start Docker containers
	docker-compose up -d

docker-down: ## Stop Docker containers
	docker-compose down

docker-logs: ## View Docker logs
	docker-compose logs -f

docker-clean: ## Remove all Docker images and containers
	docker-compose down --rmi all --volumes

# Development commands (without Docker)
dev:
	air

run:
	go run main.go

build:
	go build -o wisdom-house.exe .