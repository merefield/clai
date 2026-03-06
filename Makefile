SHELL := /bin/bash

.PHONY: lint test check

lint:
	shellcheck clai.sh install.sh tools/*.sh

test:
	bats test

check: lint test
