src = $(shell find . -type f -name '*.go' -not -path "./vendor/*")

bin/awair: $(src)
	go build -o ./bin/awair ./cmd/awair

.PHONY: setup
setup:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v1.50.1

.PHONY: lint
lint:
	./bin/golangci-lint run ./...
