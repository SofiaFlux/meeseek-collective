.PHONY: test test-race vet build

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

build:
	go build ./cmd/summa42 ./cmd/summa42-box
