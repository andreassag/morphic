# Go targets
.PHONY: build test run vet tidy hooks

tidy:
	go mod tidy

build: tidy
	go build -o ./bin/morphic ./cmd/morphic

test:
	go test ./...

vet:
	go vet ./...

hooks:
	chmod +x .githooks/*
	git config core.hooksPath .githooks

run: build
	./bin/morphic
