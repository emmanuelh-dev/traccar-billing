BINARY := traccar-billing

.PHONY: build run test lint

build:
	CGO_ENABLED=0 go build -o bin/$(BINARY) ./cmd/traccar-billing

run:
	set -a; [ -f .env ] && . ./.env; set +a; go run ./cmd/traccar-billing

test:
	go test ./...

lint:
	go vet ./...
	golangci-lint run
