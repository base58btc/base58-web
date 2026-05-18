APP_NAME = base58-website
DB_DRIVER ?= sqlite3
DB_MIGRATIONS_DIR = migrations/$(DB_DRIVER)
ifeq ($(DB_DRIVER),sqlite3)
DB_MIGRATIONS_DIR = migrations/sqlite
endif
ifeq ($(DB_DRIVER),sqlite)
DB_MIGRATIONS_DIR = migrations/sqlite
endif

.PHONY: dev-run
dev-run: build-all
	air -build.bin target/$(APP_NAME) -build.cmd="make build-all"

.PHONY: build
build:
	go build -v -o target/$(APP_NAME) ./cmd/web/main.go

.PHONY: build-all
build-all: build

.PHONY: db-status
db-status:
	go run ./cmd/migrate -dir $(DB_MIGRATIONS_DIR) -driver $(DB_DRIVER) -database "$$DATABASE_URL" status

.PHONY: db-migrate
db-migrate:
	go run ./cmd/migrate -dir $(DB_MIGRATIONS_DIR) -driver $(DB_DRIVER) -database "$$DATABASE_URL" up

.PHONY: db-rollback
db-rollback:
	go run ./cmd/migrate -dir $(DB_MIGRATIONS_DIR) -driver $(DB_DRIVER) -database "$$DATABASE_URL" down

.PHONY: clean
clean:
	rm -f target/*
