.PHONY: test test-race vet build

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

build:
	go build ./cmd/meeseek ./cmd/meeseek-box
