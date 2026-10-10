.PHONY: test run lint

test:
	go test ./...

run:
	go run ./cmd/api

lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)
