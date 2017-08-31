// apisettings package implements internal structs which configure the HTTP api.

package apisettings

import (
	"net/url"
	"path/filepath"
)

const (
	// ReverseExporterLatestApi gives the current latest API string.
	ReverseExporterLatestApi = "v1"
)

type APISettings struct {
	// ContextPath is any URL-prefix being passed by a reverse proxy.
	ContextPath string
	// StaticProxy is the URL of a proxy serving static assets.
	StaticProxy *url.URL
}

// WrapPath wraps a given URL string in the context path
func (api *APISettings) WrapPath(path string) string {
	return filepath.Join(api.ContextPath, path)
}
