.PHONY: build test

build:
	go build -o bin/gg ./cmd/gg

test:
	sh scripts/test.sh
