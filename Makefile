APP_NAME = base58-website

.PHONY: run
run:
	go run ./cmd/web/main.go

.PHONY: build
build:
	go build -o $(APP_NAME) ./cmd/web/main.go

.PHONY: all
all: build

.PHONY: clean
clean:
	rm -f $(APP_NAME)