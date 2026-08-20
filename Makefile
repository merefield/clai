SHELL := /bin/bash
BINARY := clai
PACKAGE := ./cmd/clai

.PHONY: build test vet lint integration-test legacy-test check install clean

build:
	go build -buildvcs=false -trimpath -o $(BINARY) $(PACKAGE)

test:
	go test ./...

vet:
	go vet ./...

lint:
	shellcheck install.sh clai.sh legacy/install.sh tools/*.sh

legacy-test:
	bats test/smoke.bats test/find_wild.bats

integration-test:
	bats test/install.bats

check: vet test lint integration-test legacy-test

install: build
	install -m 0755 $(BINARY) $${DESTDIR}$${PREFIX:-/usr/local}/bin/$(BINARY)

clean:
	go clean
	rm -f $(BINARY)
