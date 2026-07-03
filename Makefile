.PHONY: run build test up down

up: ## start postgres + redis
	docker compose up -d

down:
	docker compose down

build:
	go build -o bin/cms ./cmd/server

run: build
	./bin/cms

test:
	go vet ./...
	go test ./...
