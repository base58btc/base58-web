APP_NAME = base58-website
PGDATA ?= .nix-postgres/data
PGLOG ?= $(CURDIR)/.nix-postgres/postgres.log
PGSOCKETDIR ?= $(CURDIR)/.nix-postgres
PGHOST = $(PGSOCKETDIR)
PGPORT ?= 55432
PGUSER ?= base58
PGDATABASE ?= base58_dev

DB_DRIVER ?= postgres
DATABASE_URL = postgres://$(PGUSER)@localhost:$(PGPORT)/$(PGDATABASE)?host=$(PGHOST)&sslmode=disable
DB_MIGRATIONS_DIR = migrations/postgres

export DB_DRIVER
export DATABASE_URL
export PGDATA
export PGDATABASE
export PGHOST
export PGPORT
export PGUSER

.PHONY: dev-run
dev-run: build-all
	air -c .air.toml

.PHONY: build
build:
	go build -v -o target/$(APP_NAME) ./cmd/web/main.go

.PHONY: build-migrate
build-migrate:
	go build -v -o target/migrate ./cmd/migrate

.PHONY: build-all
build-all: build build-migrate

.PHONY: db-status
db-status:
	go run ./cmd/migrate -dir $(DB_MIGRATIONS_DIR) -driver $(DB_DRIVER) -database "$(DATABASE_URL)" status

.PHONY: db-migrate
db-migrate:
	go run ./cmd/migrate -dir $(DB_MIGRATIONS_DIR) -driver $(DB_DRIVER) -database "$(DATABASE_URL)" up

.PHONY: db-rollback
db-rollback:
	go run ./cmd/migrate -dir $(DB_MIGRATIONS_DIR) -driver $(DB_DRIVER) -database "$(DATABASE_URL)" down

.PHONY: postgres-init
postgres-init:
	mkdir -p $(PGSOCKETDIR)
	test -d $(PGDATA) || initdb -D $(PGDATA) -U $(PGUSER) --auth=trust --no-locale --encoding=UTF8

.PHONY: postgres-start
postgres-start: postgres-init
	mkdir -p $(PGSOCKETDIR)
	pg_ctl -D $(PGDATA) -l $(PGLOG) -o "-p $(PGPORT) -k $(PGSOCKETDIR) -c listen_addresses=''" start
	createdb -h $(PGHOST) -p $(PGPORT) -U $(PGUSER) $(PGDATABASE) || true

.PHONY: postgres-stop
postgres-stop:
	pg_ctl -D $(PGDATA) stop

.PHONY: postgres-status
postgres-status:
	pg_ctl -D $(PGDATA) status

.PHONY: postgres-reset
postgres-reset:
	dropdb -h $(PGHOST) -p $(PGPORT) -U $(PGUSER) --if-exists $(PGDATABASE)
	createdb -h $(PGHOST) -p $(PGPORT) -U $(PGUSER) $(PGDATABASE)

.PHONY: clean
clean:
	rm -f target/*
