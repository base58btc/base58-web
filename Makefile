APP_NAME = base58-website
PGDATA ?= .nix-postgres/data
PGHOST ?= 127.0.0.1
PGPORT ?= 55432
PGUSER ?= base58
PGDATABASE ?= base58_dev
PGLOG ?= $(CURDIR)/.nix-postgres/postgres.log
PGSOCKETDIR ?= $(CURDIR)/.nix-postgres

DB_DRIVER ?= postgres
DATABASE_URL ?= postgres://$(PGUSER)@$(PGHOST):$(PGPORT)/$(PGDATABASE)?sslmode=disable
DB_MIGRATIONS_DIR = migrations/postgres

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
	pg_ctl -D $(PGDATA) -l $(PGLOG) -o "-p $(PGPORT) -k $(PGSOCKETDIR)" start
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
