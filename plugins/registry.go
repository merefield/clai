// Package plugins is the explicit registration point for CLAI's compiled-in
// LLM tools. Add a constructor here after copying an existing plugin package.
package plugins

import (
	"net/http"

	"github.com/merefield/clai/pkg/tool"
	"github.com/merefield/clai/plugins/wikipedia"
)

func Registry(client *http.Client) (*tool.Registry, error) {
	return tool.NewRegistry(
		wikipedia.New(client),
	)
}
