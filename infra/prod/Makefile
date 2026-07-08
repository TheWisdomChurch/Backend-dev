.PHONY: help deploy migrate logs ps restart down

COMPOSE = docker compose -f docker-compose.yml

help:
	@echo "Wisdom House Backend — production deploy"
	@echo ""
	@echo "  make deploy    Pull latest image and (re)start the api container"
	@echo "  make migrate   Run pending migrations only, without starting the server"
	@echo "  make logs      Tail api container logs"
	@echo "  make ps        Show container status"
	@echo "  make restart   Restart the api container"
	@echo "  make down      Stop and remove containers"

# Migrations run automatically and unconditionally on every api boot
# (internal/database/migrations.go, called from cmd/api/main.go) — this
# target is for running them on demand, without starting the HTTP server.
# It uses the `migrate` subcommand baked into the app binary itself.
# Assumes the image has already been pulled (`deploy` pulls first).
migrate:
	$(COMPOSE) run --rm --entrypoint /app/wisdom-house api migrate

deploy:
	$(COMPOSE) pull
	$(MAKE) migrate
	$(COMPOSE) up -d --remove-orphans

logs:
	$(COMPOSE) logs -f api

ps:
	$(COMPOSE) ps

restart:
	$(COMPOSE) restart api

down:
	$(COMPOSE) down

.DEFAULT_GOAL := help
