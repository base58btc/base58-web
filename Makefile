APP_NAME = base58-website

.PHONY: dev-run
dev-run:
	go build -o target/$(APP_NAME) ./cmd/web/main.go
	./target/${APP_NAME} &
	./tools/tailwind -i templates/css/input.css -o static/css/styles.css --watch

.PHONY: run
run:
	./tools/tailwind -i templates/css/input.css -o static/css/styles.css --minify
	go run ./cmd/web/main.go

.PHONY: build
build:
	go build -o $(APP_NAME) ./cmd/web/main.go

.PHONY: all
all: build

.PHONY: clean
clean:
	rm -f $(APP_NAME)
