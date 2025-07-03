APP_NAME = base58-website

.PHONY: dev-run
dev-run: build-all
	air -build.bin target/$(APP_NAME) -build.cmd="make build-all"

.PHONY: build
build:
	go build -v -o target/$(APP_NAME) ./cmd/web/main.go

.PHONY: build-all
build-all: build

.PHONY: clean
clean:
	rm -f target/*
