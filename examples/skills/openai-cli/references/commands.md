# OpenAI CLI command reference

The [official models CLI reference](https://developers.openai.com/api/reference/cli/resources/models)
documents `openai models list` for fetching the models currently available to
the configured API credentials. Its response includes model identifiers and
ownership information. Use live output instead of a fixed model catalog.

The same resource reference documents model retrieval and fine-tuned model
deletion. Consult the installed command's help for arguments before proposing
either operation. Deletion must retain CLAI's normal danger-zone review.

For other resources, inspect help from the installed CLI. Do not substitute
legacy SDK CLI syntax or invent flags from another tool's documentation.
Installation and version details are in the
[official CLI repository](https://github.com/openai/openai-cli).

The model-list operation was verified against the official CLI documentation
on 27 October 2026. Installed versions may differ. Credentials must already be
configured separately; do not include secrets in commands or display them.
