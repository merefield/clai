SHELL := /bin/bash
BINARY := clai
OUTPUT ?= $(BINARY)
PACKAGE := ./cmd/clai

.PHONY: build test vet integration-test check install install-wikipedia cleanup-legacy clean

build:
	go build -buildvcs=false -trimpath -o $(OUTPUT) $(PACKAGE)

test:
	go test ./...

vet:
	go vet ./...

integration-test:
	bats test

check: vet test integration-test

install: build
	mkdir -p $${DESTDIR}$${PREFIX:-/usr/local}/bin
	install -m 0755 $(OUTPUT) $${DESTDIR}$${PREFIX:-/usr/local}/bin/$(BINARY)

install-wikipedia:
	set -eu; \
	tools_dir="$${CLAI_TOOLS_DIR:-$${XDG_CONFIG_HOME:-$$HOME/.config}/clai/tools.d}"; \
	mkdir -p "$$tools_dir"; \
	go build -buildvcs=false -trimpath -o "$$tools_dir/wikipedia" ./examples/wikipedia; \
	install -m 0600 examples/wikipedia/wikipedia.json "$$tools_dir/wikipedia.json"; \
	echo "Installed Wikipedia MCP tool in $$tools_dir"

cleanup-legacy:
	./scripts/cleanup-legacy.sh

clean:
	go clean
	rm -f $(OUTPUT)
