SHELL := /bin/bash
BINARY := clai
PACKAGE := ./cmd/clai

.PHONY: build test vet integration-test check install clean

build:
	go build -buildvcs=false -trimpath -o $(BINARY) $(PACKAGE)

test:
	go test ./...

vet:
	go vet ./...

integration-test:
	bats test/install.bats

check: vet test integration-test

install: build
	install -m 0755 $(BINARY) $${DESTDIR}$${PREFIX:-/usr/local}/bin/$(BINARY)

clean:
	go clean
	rm -f $(BINARY)
