.PHONY: help build run test lint clean migrate seed docker-build

APP_NAME=wisdomhousebe
MAIN_PATH=cmd/api/main.go
BUILD_DIR=bin
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

help:
	@echo "Wisdom House Backend"
	@echo ""
	@echo "Development:"
	@echo "  make run              Run server"
	@echo "  make watch            Run with auto-reload"
	@echo "  make test             Run tests"
	@echo "  make test-coverage    Tests with coverage"
	@echo "  make lint             Run linter"
	@echo ""
	@echo "Build:"
	@echo "  make build            Build binary"
	@echo "  make build-prod       Optimized build"
	@echo "  make clean            Clean artifacts"
	@echo ""
	@echo "Database:"
	@echo "  make migrate          Run migrations"
	@echo "  make seed             Seed test data"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-build     Build Docker image"

run:
	go run $(MAIN_PATH)

watch:
	air

build: clean
	go build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PATH)
	@echo "✅ Built: $(BUILD_DIR)/$(APP_NAME)"

build-prod: clean
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
		-o $(BUILD_DIR)/$(APP_NAME) \
		-ldflags "-w -s -X main.Version=$(VERSION)" $(MAIN_PATH)
	@echo "✅ Optimized binary built"

test:
	go test -v -race -timeout 30s ./...

test-coverage:
	go test -v -race -coverprofile=.coverage.out ./...
	go tool cover -html=.coverage.out -o .coverage.html
	@echo "✅ Coverage: .coverage.html"

lint:
	go fmt ./...
	go vet ./...
	@echo "✅ Code formatted and vetted"

clean:
	rm -rf $(BUILD_DIR)
	rm -f .coverage.out .coverage.html
	go clean
	@echo "✅ Cleaned"

migrate:
	go run $(MAIN_PATH) migrate

seed:
	go run $(MAIN_PATH) seed

docker-build:
	docker build -t wisdom-house-backend:latest -f dockerfile .
	@echo "✅ Docker image built"

.DEFAULT_GOAL := help
