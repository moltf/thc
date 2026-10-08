BINARY=thc
VERSION=1.0.0

.PHONY: build test vet clean

build:
	gofmt -w .
	go build -trimpath -ldflags="-s -w" -o $(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)
