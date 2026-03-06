#!/bin/bash
# -*- Mode: sh; coding: utf-8; indent-tabs-mode: t; tab-width: 4 -*-

# Enables CLAI to search for files and directories and forces wildcards

init() {
	echo '{
		"type": "function",
		"function": {
			"name": "find-wildcard",
			"description": "Use this to find any file or directory.",
			"parameters": {
				"type": "object",
				"properties": {
					"path": {
						"type": "string",
						"description": "The path to search recursivly from"
					},
					"name": {
						"type": "string",
						"description": "The iname to search for"
					}
				},
				"required": [
					"path",
					"name"
				]
			}
		}
	}'
}

execute() {
	local path
	local name
	local output
	path=$(echo "$1" | jq -r '.path')
	name=$(echo "$1" | jq -r '.name')
	if [ ! -d "$path" ]; then
		echo "Not found"
		return 0
	fi

	output=$(find "$path" -iname "*$name*" 2>/dev/null)
	if [ -n "$output" ]; then
		output=$(echo "$output" | awk '{printf "%s\\n", $0}')
		echo "$output"
	else
		echo "Not found"
	fi
}
