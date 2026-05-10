.PHONY: help up down restart build test test-integration test-performance \
       logs db-flush redis-flush loki-flush flush clean \
       db-shell redis-shell db-migrate

DC = docker compose

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ──────────────────────────────────────────────
#  Docker Compose
# ──────────────────────────────────────────────

up: ## Start all services
	$(DC) up -d
	@echo "Waiting for services to be ready..."
	@sleep 5
	@echo "Services running. Grafana: http://localhost:3000"

down: ## Stop all services (keep data)
	$(DC) down

restart: ## Restart all services
	$(DC) restart

build: ## Build and start services
	$(DC) up -d --build

# ──────────────────────────────────────────────
#  Testing
# ──────────────────────────────────────────────

test: ## Run unit tests
	go test -v -count=1 ./...

test-integration: ## Run integration tests (requires services running)
	go test -v -count=1 -tags=integration -timeout=120s ./tests/integration/

test-performance: ## Run performance/load tests
	bash tests/performance/run.sh

# ──────────────────────────────────────────────
#  Data Flush / Cleanup
# ──────────────────────────────────────────────

flush: db-flush redis-flush loki-flush ## Flush DB, Redis, and Loki logs

db-flush: ## Truncate all application tables
	docker exec postgres_db psql -U samil -d myappdb -c \
		"TRUNCATE notifications, batches CASCADE"

redis-flush: ## Flush all Redis data
	docker exec redis_cache redis-cli FLUSHALL

loki-flush: ## Delete Loki logs and restart
	$(DC) stop loki
	docker volume rm notification_loki_data 2>/dev/null || true
	$(DC) up -d loki

clean: ## Full reset: stop services, remove all volumes, rebuild
	$(DC) down -v
	@echo "All data volumes removed. Run 'make up' or 'make build' for a fresh start."

# ──────────────────────────────────────────────
#  CLI / Debug
# ──────────────────────────────────────────────

db-shell: ## Open psql shell
	docker exec -it postgres_db psql -U samil -d myappdb

redis-shell: ## Open redis-cli shell
	docker exec -it redis_cache redis-cli

db-migrate: ## Run database migrations
	go run -tags=integration ./cmd/server/main.go 2>/dev/null || \
		echo "Tip: Migrations run automatically on server startup."

logs: ## Tail logs for server and worker
	$(DC) logs -f server worker