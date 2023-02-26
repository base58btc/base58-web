.PHONY: run
run:
	go run ./cmd/web/main.go

.PHONY: build
build:
	go build -o main ./cmd/web/main.go

.PHONY: all
all: build
