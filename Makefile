BINARY := devmachine
VERSION ?= dev

.PHONY: build test test-vps vps-up vps-down fmt lint run

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/devmachine

test:
	go test -race ./...

# The integration tests skip without a machine to reach. This starts the
# throwaway VPS and points them at it.
test-vps: vps-up
	eval "$$(scripts/fake-vps.sh env)" && go test -race ./...

vps-up:
	scripts/fake-vps.sh up

vps-down:
	scripts/fake-vps.sh down

fmt:
	gofmt -w .

lint:
	golangci-lint run

run: build
	./$(BINARY) $(ARGS)
