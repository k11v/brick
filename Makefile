.PHONY: generate
generate:
	go generate ./...

.PHONY: fix
fix:
	go mod tidy
	golangci-lint run --fix ./...

.PHONY: check
check:
	golangci-lint run ./...

.PHONY: test
test:
	go test -v ./...
