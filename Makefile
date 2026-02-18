.PHONY: all build test test-contracts test-gateway test-client test-integration clean

all: build

# Build
build: build-gateway build-client

build-gateway:
	cd gateway && go build -o ../bin/sovereign-gateway ./cmd/gateway

build-client:
	cd client && go build -o ../bin/svpn ./cmd/svpn

# Test
test: test-contracts test-gateway test-client test-integration

test-contracts:
	cd contracts && forge test -vvv

test-gateway:
	cd gateway && go test -race -count=1 ./...

test-client:
	cd client && go test -race -count=1 ./...

test-integration:
	cd integration && go test -race -v -count=1 ./...

# Docker
docker-build:
	docker build -t sovereign-vpn-gateway ./gateway
	docker build -t sovereign-vpn-client ./client

# Clean
clean:
	rm -rf bin/
	cd contracts && forge clean

# Dev helpers
keygen:
	cd client && go run ./cmd/svpn keygen

health:
	curl -s http://localhost:8080/health | jq .
