default: test

test:
	go vet ./...
	staticcheck ./...
	go test -vet=off -race -coverprofile=coverage.txt -covermode=atomic ./...

tests: test

build:
	go build ./cmd/gh-find
	go build ./cmd/gh-go-rdeps
	go build ./cmd/gh-pr
	go build ./cmd/gh-purge-artifacts
	go build ./cmd/gh-watch

.PHONY: test tests build
