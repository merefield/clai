---
name: openai-cli
description: >-
  Use the official OpenAI REST API CLI (openai) to inspect models, files,
  responses, audio and other API resources. Activate for OpenAI CLI commands
  or requests to list available OpenAI API models and work with API resources
  from the terminal.
compatibility: Requires the official openai executable and separately configured API credentials.
metadata:
  source: https://github.com/openai/openai-cli
---

# OpenAI CLI

Use `openai` for the official REST API CLI. `codex` is a separate coding agent.

For a request to list the models available to the user's API credentials,
propose `openai models list`. Obtain live results through CLAI's existing
command review and execution flow; do not invent account access or model IDs.

Check `openai --help` and the relevant subcommand's help before using syntax
that may differ between installed versions. Never invent commands or flags.
Use [the command reference](references/commands.md) only when further detail
is needed. Reference reading is optional; the basic model-list command is here.

Keep credentials out of command output, arguments, explanations and history.
Use the user's existing credential configuration; never print API keys.
Follow CLAI's approval rules and the project's applicable AGENTS guidance.
Reading this skill does not authorize scripts, network tools, or execution.
