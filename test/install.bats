#!/usr/bin/env bats

setup() {
  export TEST_ROOT
  TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/clai-install.XXXXXX")"
  mkdir -p "$TEST_ROOT/fakebin"
  mkdir -p "$TEST_ROOT/bin"
  mkdir -p "$TEST_ROOT/lib"
}

teardown() {
  rm -rf "$TEST_ROOT"
}

@test "install script reports download failures cleanly" {
  cat > "$TEST_ROOT/fakebin/curl" <<'EOF'
#!/bin/bash
exit 22
EOF
  chmod +x "$TEST_ROOT/fakebin/curl"

  run env \
    PATH="$TEST_ROOT/fakebin:$PATH" \
    CLAI_BIN_DIR="$TEST_ROOT/bin" \
    bash ./install.sh

  [ "$status" -eq 1 ]
  [[ "$output" == *"Failed to download clai.sh"* ]]
}

@test "install script installs a real script and symlink into an overridden bin dir" {
  cat > "$TEST_ROOT/fakebin/curl" <<'EOF'
#!/bin/bash
output=""
while [ $# -gt 0 ]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
printf '%s\n' '#!/bin/bash' 'echo installed clai' > "$output"
EOF
  chmod +x "$TEST_ROOT/fakebin/curl"

  cat > "$TEST_ROOT/fakebin/sudo" <<'EOF'
#!/bin/bash
"$@"
EOF
  chmod +x "$TEST_ROOT/fakebin/sudo"

  run env \
    PATH="$TEST_ROOT/fakebin:$PATH" \
    CLAI_INSTALL_DIR="$TEST_ROOT/lib/clai" \
    CLAI_BIN_DIR="$TEST_ROOT/bin" \
    bash ./install.sh

  [ "$status" -eq 0 ]
  [ -f "$TEST_ROOT/lib/clai/clai.sh" ]
  [ -x "$TEST_ROOT/lib/clai/clai.sh" ]
  [ -L "$TEST_ROOT/bin/clai" ]
  [ -x "$TEST_ROOT/bin/clai" ]
  run "$TEST_ROOT/bin/clai"
  [ "$status" -eq 0 ]
  [ "$output" = "installed clai" ]
}

@test "install script fails when the exposed symlink target is not executable" {
  cat > "$TEST_ROOT/fakebin/curl" <<'EOF'
#!/bin/bash
output=""
while [ $# -gt 0 ]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
printf '%s\n' '#!/bin/bash' 'echo installed clai' > "$output"
EOF
  chmod +x "$TEST_ROOT/fakebin/curl"

  cat > "$TEST_ROOT/fakebin/sudo" <<'EOF'
#!/bin/bash
"$@"
EOF
  chmod +x "$TEST_ROOT/fakebin/sudo"

  cat > "$TEST_ROOT/fakebin/ln" <<'EOF'
#!/bin/bash
dest="${@: -1}"
rm -f "$dest"
/bin/ln -s /nonexistent/clai.sh "$dest"
EOF
  chmod +x "$TEST_ROOT/fakebin/ln"

  run env \
    PATH="$TEST_ROOT/fakebin:$PATH" \
    CLAI_INSTALL_DIR="$TEST_ROOT/lib/clai" \
    CLAI_BIN_DIR="$TEST_ROOT/bin" \
    bash ./install.sh

  [ "$status" -eq 1 ]
  [[ "$output" == *"Failed to create CLAI symlink in $TEST_ROOT/bin"* ]]
}
