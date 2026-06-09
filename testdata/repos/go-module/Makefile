.PHONY: build test fmt

build:
	go build ./...

test:
	go test ./...

fmt:
	gofmt -w .

migrate:
	psql -d local -f schema.sql

proto:
	protoc --proto_path=proto api.proto
