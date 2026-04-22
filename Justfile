# Foyer — self-hosted server dashboard

# Run frontend dev server + Go backend concurrently
dev:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p /tmp/foyer-dev/files
    trap 'kill 0' EXIT
    cd frontend && pnpm dev --port 5173 --host 0.0.0.0 &
    FOYER_DEV=1 go run . --config dev.json --jwt-secret-file .dev-jwt-secret --port 8420
    wait

# Go backend only (no frontend, for when you run pnpm dev separately)
backend:
    mkdir -p /tmp/foyer-dev/files
    FOYER_DEV=1 go run . --config dev.json --jwt-secret-file .dev-jwt-secret --port 8420

# Frontend only
frontend:
    cd frontend && pnpm dev --port 5173 --host 0.0.0.0

# Build frontend, then build Go binary with embedded frontend
build:
    cd frontend && pnpm install --frozen-lockfile && pnpm build
    CGO_ENABLED=0 go build -o foyer .

# Build for harbor (x86_64-linux)
build-amd64:
    cd frontend && pnpm install --frozen-lockfile && pnpm build
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o foyer-linux-amd64 .

# Build for raven (aarch64-linux)
build-arm64:
    cd frontend && pnpm install --frozen-lockfile && pnpm build
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o foyer-linux-arm64 .

# Run Go tests
test:
    go test ./...

# Clean build artifacts
clean:
    rm -rf foyer foyer-* frontend/build frontend/.svelte-kit
