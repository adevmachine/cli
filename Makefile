BINARY := devmachine
VERSION ?= dev

.PHONY: build surface settings test test-vps cover vps-up vps-down fmt lint docs run

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/devmachine

# SURFACE.txt is committed, so a change to the command surface shows up in a
# diff rather than passing unnoticed.
surface:
	go run ./cmd/surface > SURFACE.txt

test:
	go test -race ./...

# The floor, not the target. It is set at a number the tree already clears, so
# it catches a regression instead of teaching everybody to ignore a red gate.
# Raise it when the number earns it.
COVERAGE_FLOOR := 80

cover:
	@go test -coverprofile=coverage.out ./internal/... >/dev/null
	@go tool cover -func=coverage.out | tail -1
	@go tool cover -func=coverage.out | tail -1 | awk '{gsub(/%/,"",$$3); \
		if ($$3+0 < $(COVERAGE_FLOOR)) { \
			printf "coverage %s%% is below the %d%% floor\n", $$3, $(COVERAGE_FLOOR); exit 1 }}'

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

# The settings page is generated from the packages' own manifests, for the same
# reason SURFACE.txt is: a page kept by hand beside the thing it describes
# drifts. PACKAGES says where that repository is checked out.
PACKAGES ?= $(HOME)/dev/packages

settings:
	go run ./cmd/settings $(PACKAGES)/packages > docs/reference/settings.md

# Every link resolves, and every page is reachable from the index.
docs:
	scripts/check-docs.sh

run: build
	./$(BINARY) $(ARGS)
