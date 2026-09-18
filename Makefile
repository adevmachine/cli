BINARY := devmachine
VERSION ?= dev

.PHONY: build surface test test-vps vps-up vps-down fmt lint docs run

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/devmachine

# SURFACE.txt is committed, so a change to the command surface shows up in a
# diff rather than passing unnoticed.
surface:
	go run ./cmd/surface > SURFACE.txt

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

# Every link resolves, and every page is reachable from the index.
docs:
	scripts/check-docs.sh

run: build
	./$(BINARY) $(ARGS)
