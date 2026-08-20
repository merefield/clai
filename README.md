# CLAI

CLAI is an AI-powered terminal assistant that turns natural-language requests into reviewed shell commands.

This `go_migration` branch contains the compatibility-first Go implementation. The previous Bash implementation remains available as [`clai.sh`](clai.sh), with its original documentation archived at [`docs/legacy-bash.md`](docs/legacy-bash.md), while parity work continues.

## Status

The port currently implements the core CLAI workflow:

- direct requests such as `clai list files by size`
- interactive sessions with `clai`
- OpenAI and generic OpenAI-compatible Chat Completions endpoints
- Ollama `/api/chat`, Anthropic Messages, and Gemini `generateContent`
- structured `cmd`, `info`, `risk`, and `variables` responses
- command review, editing, risk appetite, and danger-zone confirmation
- Bash command execution with live stdout and stderr
- compatible `~/.clai_tools/*.sh` plugins through a Bash adapter
- compatible configuration and persisted history formats
- setup, history, history clearing, and command-result sharing controls

The Go implementation is not yet declared a byte-for-byte UI replacement. Applicable legacy Bats scenarios run alongside the Go tests, and the installer suite now exercises the Go-binary installation path.

## Requirements

- Go 1.26.6 for building; `go.mod` declares the toolchain
- Bash at runtime for suggested command execution and legacy shell plugins

HTTP and JSON are implemented with Go's standard library, so runtime installations do not require `curl` or `jq`.

## Build and run

```bash
make check
make build
./clai setup
./clai how much is 3 times pi
```

Install the locally built command as `clai`:

```bash
./install.sh
```

The default destination is `/usr/local/bin/clai`. Override it with `CLAI_BIN_DIR` or `CLAI_BIN_NAME`.

## Shell parsing

Ordinary prompts do not need surrounding quotes:

```bash
clai create a directory named reports
```

The parent shell still interprets glob and syntax characters before CLAI receives the arguments. Use words, escaping, quoting, or shell-specific `noglob` integration when needed:

```bash
clai how much is 3 times pi
clai how much is 3 \* pi
clai "how much is 3 * pi"
```

For the exact unquoted form, install CLAI's shell integration in your shell profile:

```bash
# ~/.bashrc
eval "$(clai shell-init bash)"
```

```zsh
# ~/.zshrc
eval "$(clai shell-init zsh)"
```

After starting a new shell, glob characters following `clai` are passed literally:

```bash
clai how much is 3 * pi
```

## Compatibility paths

By default, CLAI deliberately reads the existing CLAI-compatible locations:

- config: `~/.config/clai.cfg`
- history: `${XDG_STATE_HOME:-~/.local/state}/clai/history_com.json`
- plugins: `~/.clai_tools`

Override any path when testing against isolated state:

```bash
export CLAI_CONFIG="/tmp/clai-go/config.cfg"
export CLAI_HISTORY="/tmp/clai-go/history.json"
export CLAI_TOOLS_DIR="/tmp/clai-go/tools"
```

## Commands

```text
clai setup
clai --setup
clai --show-history [--verbose]
clai --clear-history
clai --show-results-sharing
clai --toggle-results-sharing
clai shell-init bash|zsh
clai --version
clai --help
```

## Architecture

The entry point uses Cobra while keeping free-form request arguments intact. Lip Gloss owns terminal styling. Core behaviour is split into small packages:

```text
cmd/clai         command entry point
internal/app     orchestration, prompts, risk and response handling
internal/config  compatible key=value configuration
internal/history JSON history persistence and retention
internal/provider native HTTP adapters for supported providers
internal/plugin  legacy Bash plugin discovery and execution
internal/runner  reviewed Bash command execution and capture
internal/ui      inline terminal rendering and input
```

Provider, command-runner, and terminal boundaries are interfaces so tests can exercise orchestration without network calls or executing suggested commands.

## Framework choices

- [Cobra](https://github.com/spf13/cobra) provides command lifecycle, help, and version behaviour.
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) provides declarative terminal styling.
- Go's `net/http` and `encoding/json` remain explicit because provider payloads differ meaningfully and do not benefit from a generic REST wrapper.
- A full-screen Bubble Tea application is intentionally deferred: the migration target is CLAI's existing inline workflow, not a UI redesign.

## Migration principles

1. Keep configuration, history, prompts, risk semantics, and legacy tools compatible.
2. Preserve the `clai request words here` interface throughout migration.
3. Treat the existing Bash/Bats behaviour as the contract.
4. Add Go unit tests at each typed boundary and black-box parity tests around the final binary.
5. Keep the legacy implementation available until behavioural parity is demonstrated.
