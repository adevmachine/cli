BINARY := devmachine
VERSION ?= dev

.PHONY: build test fmt lint run

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/devmachine

test:
	go test -race ./...

fmt:
	gofmt -w .

lint:
	golangci-lint run

run: build
	./$(BINARY) $(ARGS)
