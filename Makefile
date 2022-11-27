src = $(shell find . -type f -name '*.go' -not -path "./vendor/*")

bin/awair: $(src)
	go build -o ./bin/awair ./cmd/awair
