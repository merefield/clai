#!/bin/sh
set -eu

bin_dir=${CLAI_BIN_DIR:-/usr/local/bin}
binary_name=${CLAI_BIN_NAME:-clai}
target=${bin_dir}/${binary_name}
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/clai-install.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

echo "Building CLAI with the Go toolchain declared in go.mod..."
go build -buildvcs=false -trimpath -o "${tmp_dir}/${binary_name}" ./cmd/clai

if [ -w "$bin_dir" ] || { [ ! -e "$bin_dir" ] && [ -w "$(dirname "$bin_dir")" ]; }; then
	mkdir -p "$bin_dir"
	install -m 0755 "${tmp_dir}/${binary_name}" "$target"
else
	sudo mkdir -p "$bin_dir"
	sudo install -m 0755 "${tmp_dir}/${binary_name}" "$target"
fi

echo "Installed ${binary_name} to ${target}"
echo "Run: ${binary_name} how much is 3 times pi"

legacy_payload=${CLAI_LEGACY_INSTALL_DIR:-/usr/local/lib/clai}/clai.sh
if [ -e "$legacy_payload" ]; then
	echo "Legacy Bash payload detected at ${legacy_payload}."
	echo "Review obsolete files with: ./scripts/cleanup-legacy.sh"
fi
