// Package tool defines the small, provider-independent contract implemented by
// tools that CLAI exposes to an LLM.
package tool

import (
	"context"
	"encoding/json"
)

// Capability describes the local authority a tool implementation needs. It is
// CLAI-side metadata and is not included in the provider function schema.
type Capability string

const (
	CapabilityNetworkRead Capability = "network-read"
	CapabilityLocalRead   Capability = "local-read"
	CapabilityLocalWrite  Capability = "local-write"
)

// Definition is the provider-independent description of an LLM-facing tool.
type Definition struct {
	Name         string
	Description  string
	Parameters   map[string]any
	Capabilities []Capability
}

// Tool is implemented by a compiled-in CLAI tool. Implementations should
// decode arguments into a typed struct and return a JSON-marshalable result.
type Tool interface {
	Definition() Definition
	Execute(context.Context, json.RawMessage) (any, error)
}
