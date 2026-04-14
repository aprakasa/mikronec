.PHONY: swag swag-fmt build test run clean lint fmt

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

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
