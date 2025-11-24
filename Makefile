# Qingping CGDN1 Collector - Makefile

# Image configuration
IMAGE_NAME := ghcr.io/mike1808/qingping-air-monitor-lite-collector
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LATEST_TAG := latest

# Go configuration
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED := 0

# Docker compose
COMPOSE_FILE := docker-compose.yml

.PHONY: help
help: ## Show this help message
	@echo "Qingping CGDN1 Collector - Available targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

.PHONY: build
build: ## Build Go binary locally
	@echo "Building Go binary..."
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o bin/qingping-collector .
	@echo "✓ Binary built: bin/qingping-collector"

.PHONY: test
test: ## Run tests
	@echo "Running tests..."
	go test -v ./...

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	@echo "✓ Clean complete"

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "Building Docker image: $(IMAGE_NAME):$(VERSION)"
	docker build -t $(IMAGE_NAME):$(VERSION) .
	docker tag $(IMAGE_NAME):$(VERSION) $(IMAGE_NAME):$(LATEST_TAG)
	@echo "✓ Docker image built:"
	@echo "  - $(IMAGE_NAME):$(VERSION)"
	@echo "  - $(IMAGE_NAME):$(LATEST_TAG)"

.PHONY: docker-build-no-cache
docker-build-no-cache: ## Build Docker image without cache
	@echo "Building Docker image (no cache): $(IMAGE_NAME):$(VERSION)"
	docker build --no-cache -t $(IMAGE_NAME):$(VERSION) .
	docker tag $(IMAGE_NAME):$(VERSION) $(IMAGE_NAME):$(LATEST_TAG)
	@echo "✓ Docker image built"

.PHONY: docker-push
docker-push: ## Push Docker image to registry
	@echo "Pushing Docker image to registry..."
	docker push $(IMAGE_NAME):$(VERSION)
	docker push $(IMAGE_NAME):$(LATEST_TAG)
	@echo "✓ Images pushed:"
	@echo "  - $(IMAGE_NAME):$(VERSION)"
	@echo "  - $(IMAGE_NAME):$(LATEST_TAG)"

.PHONY: docker-login
docker-login: ## Login to GitHub Container Registry
	@echo "Logging in to ghcr.io..."
	@echo "Please enter your GitHub Personal Access Token (PAT) with packages:write scope"
	docker login ghcr.io -u mike1808

.PHONY: release
release: docker-build docker-push ## Build and push Docker image

.PHONY: up
up: ## Start services with docker-compose
	@echo "Starting services..."
	docker-compose -f $(COMPOSE_FILE) up -d
	@echo "✓ Services started"
	@echo "View logs: make logs"

.PHONY: down
down: ## Stop services
	@echo "Stopping services..."
	docker-compose -f $(COMPOSE_FILE) down
	@echo "✓ Services stopped"

.PHONY: restart
restart: down up ## Restart services

.PHONY: logs
logs: ## Show container logs (follow mode)
	docker-compose -f $(COMPOSE_FILE) logs -f qingping-collector

.PHONY: logs-all
logs-all: ## Show all container logs
	docker-compose -f $(COMPOSE_FILE) logs -f

.PHONY: ps
ps: ## Show running containers
	docker-compose -f $(COMPOSE_FILE) ps

.PHONY: shell
shell: ## Open shell in running container
	docker-compose -f $(COMPOSE_FILE) exec qingping-collector sh

.PHONY: rebuild
rebuild: ## Rebuild and restart services
	@echo "Rebuilding services..."
	docker-compose -f $(COMPOSE_FILE) down
	docker-compose -f $(COMPOSE_FILE) build
	docker-compose -f $(COMPOSE_FILE) up -d
	@echo "✓ Services rebuilt and restarted"

.PHONY: metrics
metrics: ## Show Prometheus metrics
	@curl -s http://localhost:9273/metrics | grep -E '^qingping_' || echo "Metrics endpoint not available"

.PHONY: check-mqtt
check-mqtt: ## Check MQTT messages (requires mosquitto-clients)
	@echo "Subscribing to MQTT topic (Ctrl+C to stop)..."
	@docker exec -it mosquitto mosquitto_sub -t 'qingping/#' -v

.PHONY: update-deps
update-deps: ## Update Go dependencies
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy
	@echo "✓ Dependencies updated"

.PHONY: version
version: ## Show version information
	@echo "Version: $(VERSION)"
	@echo "Image:   $(IMAGE_NAME)"
	@echo "Tags:    $(VERSION), $(LATEST_TAG)"

.PHONY: install-deps
install-deps: ## Install Go dependencies
	@echo "Installing dependencies..."
	go mod download
	@echo "✓ Dependencies installed"

# Development helpers
.PHONY: dev-up
dev-up: rebuild logs ## Quick development cycle: rebuild and show logs

.PHONY: status
status: ps metrics ## Show status and metrics

# Default target
.DEFAULT_GOAL := help
