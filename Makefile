# Go & Docker targets
.PHONY: build test run vet tidy hooks docker-up docker-down docker-logs docker-build dev automerge

automerge:
	./.github/scripts/dependabot-automerge.sh

tidy:
	go mod tidy

build: tidy
	go build -o ./bin/morphic ./cmd/morphic

test:
	go test -v ./...

vet:
	go vet ./...

hooks:
	chmod +x .githooks/*
	git config core.hooksPath .githooks

run: build
	./bin/morphic

dev: build
	DATABASE_URL="postgres://morphic:morphic_secret@localhost:5432/morphic?sslmode=disable" ./bin/morphic

docker-build:
	docker compose build

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f
