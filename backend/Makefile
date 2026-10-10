.PHONY: test run lint migrate

test:
	go test ./...

run:
	go run ./cmd/api

migrate:
	go run ./cmd/migrate

lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)
