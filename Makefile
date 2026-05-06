.PHONY: build run test lint fmt clean

build:
	cd src && go build -o ../bin/gateway ./cmd/gateway/

run:
	cd src && go run ./cmd/gateway/

test:
	cd src && go test ./...

test-verbose:
	cd src && go test -v ./...

lint:
	cd src && go vet ./...

fmt:
	cd src && gofmt -w .

clean:
	rm -rf bin/

tidy:
	cd src && go mod tidy
