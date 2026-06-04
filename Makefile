.PHONY: test lint run vet vuln build format check-all

test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

lint:
	golangci-lint run ./...

run:
	go run cmd/server/main.go

vet:
	go vet ./...
	
vuln:
	govulncheck ./...

build:
	CGO_ENABLED=1 go build -o bin/server cmd/server/main.go

format:
	goimports -w .
	go mod tidy

check-all: format lint vuln test