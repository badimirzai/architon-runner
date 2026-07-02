.PHONY: fmt vet test race lint build clean

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

lint:
	golangci-lint run

build:
	go build -o bin/architon ./cmd/architon

clean:
	rm -rf bin/
