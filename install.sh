#!/bin/bash
# -*- Mode: sh; coding: utf-8; indent-tabs-mode: t; tab-width: 4 -*-

REPO_OWNER="${CLAI_REPO_OWNER:-merefield}"
REPO_NAME="${CLAI_REPO_NAME:-clai}"
REPO_BRANCH="${CLAI_REPO_BRANCH:-main}"
REPO_SCRIPT="${CLAI_REPO_SCRIPT:-clai.sh}"
SCRIPT_URL="${CLAI_SCRIPT_URL:-https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/${REPO_BRANCH}/${REPO_SCRIPT}}"
INSTALL_DIR="${CLAI_INSTALL_DIR:-/usr/local/lib/clai}"
BIN_DIR="${CLAI_BIN_DIR:-/usr/local/bin}"
BIN_NAME="${CLAI_BIN_NAME:-clai}"
INSTALL_PATH="${INSTALL_DIR}/${REPO_SCRIPT}"
LINK_PATH="${BIN_DIR}/${BIN_NAME}"
TMP_FILE=$(mktemp)

# Download the script file
echo
echo "Downloading CLAI..."
curl -q --fail --location --progress-bar --output "$TMP_FILE" "$SCRIPT_URL"
ret=$?
echo

# Check if curl succeeded
if [ $ret -ne 0 ]; then
	echo "Failed to download $REPO_SCRIPT from $SCRIPT_URL"
	exit 1
fi
if [ ! -f "$TMP_FILE" ]; then # curl succeeded but file doesn't exist
	echo "Failed to create $TMP_FILE"
	exit 1
fi
if [ ! -s "$TMP_FILE" ]; then # file exists but is empty
	echo "Downloaded $TMP_FILE is empty"
	exit 1
fi

# Install the real script to a stable location and expose it via a symlink on PATH
echo "Installing CLAI script to $INSTALL_DIR..."
sudo mkdir -p "$INSTALL_DIR"
sudo mv "$TMP_FILE" "$INSTALL_PATH"
if [ ! -f "$INSTALL_PATH" ]; then
	echo "Failed to install CLAI to $INSTALL_DIR"
	exit 1
fi

# Make the script executable
sudo chmod +x "$INSTALL_PATH"
if [ ! -x "$INSTALL_PATH" ]; then
	echo "Failed to make CLAI executable"
	exit 1
fi

echo "Creating symlink in $BIN_DIR..."
sudo mkdir -p "$BIN_DIR"
sudo ln -sf "$INSTALL_PATH" "$LINK_PATH"
if [ ! -L "$LINK_PATH" ] || [ ! -x "$LINK_PATH" ]; then
	echo "Failed to create CLAI symlink in $BIN_DIR"
	exit 1
fi

# Done!
echo
echo "Installation completed successfully!"
echo "Run '${BIN_NAME}' to start CLAI (you may need to restart your terminal)"
echo "Installed script: $INSTALL_PATH"
echo "Command symlink: $LINK_PATH"
echo "Visit https://github.com/${REPO_OWNER}/${REPO_NAME} for more information"
exit 0
