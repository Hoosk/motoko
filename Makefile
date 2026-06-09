.PHONY: test lint run vet vuln build format check-all

test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

lint:
	golangci-lint run ./...

run:
	go run ./cmd/motoko

vet:
	go vet ./...
	
vuln:
	govulncheck ./...

build:
	CGO_ENABLED=1 go build -o motoko ./cmd/motoko

format:
	goimports -w .
	go mod tidy

check-all: format lint vuln test