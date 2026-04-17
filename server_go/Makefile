.PHONY: build test vet lint check tools

# Path where `go install` places binaries.
GOBIN ?= $(shell go env GOPATH)/bin

build:
	go build ./...

test:
	go test ./... -count=1

vet:
	go vet ./...

# Install linter if missing. Pinned through go.mod would be cleaner; the
# version below matches the config we validated against.
tools:
	@test -x "$(GOBIN)/golangci-lint" || \
		GOBIN=$(GOBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8

lint: tools
	$(GOBIN)/golangci-lint run ./...

# Aggregate gate suitable for pre-commit / CI.
check: vet lint test
