.PHONY: generate
generate:
	go generate ./...

.PHONY: fix
fix:
	go mod tidy
	go run github.com/swaggo/swag/cmd/swag fmt -g cmd/server/main.go
	golangci-lint run --fix ./...

.PHONY: check
check:
	golangci-lint run ./...

.PHONY: test
test:
	go test -v ./...
