.PHONY: swag swag-fmt build test run clean setup lint fmt

setup:
	@which pre-commit > /dev/null || pip install pre-commit
	pre-commit install
	@echo "Pre-commit hooks installed successfully!"

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w .

swag:
	swag init -g cmd/server/main.go -o ./docs --parseDependency --parseInternal

swag-fmt:
	swag fmt -g cmd/server/main.go -d ./internal/handler,./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./...

run:
	go run ./cmd/server

clean:
	rm -rf bin/
