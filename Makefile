BINARY := presenca-facial
CMD     := ./cmd/presenca-facial
MIGRATIONS_DIR := ./migrations
DB_URL  ?= postgres://presenca:presenca_dev@localhost:5432/presenca_facial?sslmode=disable

.PHONY: build test test-integration lint migrate-up migrate-down quickstart dev docker-up docker-down

## build: compile the binary
build:
	go build -o bin/$(BINARY) $(CMD)

## test: run all unit tests (no external dependencies)
test:
	go test ./... -v -count=1

## test-integration: run integration tests against live PostgreSQL + RabbitMQ
## Requires: docker compose up (or set TEST_DATABASE_URL + TEST_RABBITMQ_URL env vars)
test-integration:
	TEST_DATABASE_URL="$(DB_URL)" \
	go test ./... -tags integration -v -count=1

## lint: run vet and staticcheck
lint:
	go vet ./...
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed; skipping"

## migrate-up: apply all pending migrations
migrate-up:
	@which migrate > /dev/null 2>&1 || (echo "golang-migrate CLI not found; install via: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest" && exit 1)
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" up

## migrate-down: rollback last migration
migrate-down:
	@which migrate > /dev/null 2>&1 || (echo "golang-migrate CLI not found" && exit 1)
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" down 1

## docker-up: start PostgreSQL and RabbitMQ
docker-up:
	docker compose up -d --wait postgres rabbitmq

## docker-down: stop and remove containers
docker-down:
	docker compose down

## quickstart: bring up dependencies, apply migrations, and build
quickstart:
	$(MAKE) docker-up
	$(MAKE) migrate-up
	$(MAKE) build
	@echo "Run: ./bin/$(BINARY) to start the service"
	@echo "Env vars required: GOB_STATE_URL, GOB_STATE_TOKEN, ADMIN_TOKEN, WEBHOOK_PATH_SECRET"
	@echo "Optional: ISAPI_DEVICE_0_HOST, ISAPI_DEVICE_0_USERNAME, ISAPI_DEVICE_0_PASSWORD"

## dev: quickstart + run the service (requires env vars to be exported)
dev: quickstart
	./bin/$(BINARY)
