.PHONY: all build clean docker test

all: build

build:
	go build -o sandbox ./cmd/sandbox
	go build -o sandbox-cli ./cmd/sandbox-cli

clean:
	rm -f sandbox sandbox-cli

docker:
	cd .. && docker build -f sandbox/Dockerfile -t redis-fs-sandbox .

test:
	go test ./...

