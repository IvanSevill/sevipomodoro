.PHONY: build run test fmt check

build:
	go build -o bin/sevipomodoro ./cmd/sevipomodoro

run:
	go run ./cmd/sevipomodoro

test:
	go test ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

check:
	gofmt -l $$(find . -name '*.go' -not -path './vendor/*')
	go vet ./...
	go test ./...
	go build ./cmd/sevipomodoro
