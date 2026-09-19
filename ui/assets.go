package ui

import (
	"net/http"
	"path"
	"strings"

	"github.com/miroslav-matejovsky/adsb-testbench/ui/internal/assets"
)

// contentTypes maps every embedded file extension to its media type.
var contentTypes = map[string]string{
	".js":   "text/javascript; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".html": "text/html; charset=utf-8",
	".txt":  "text/plain; charset=utf-8",
	".png":  "image/png",
}

// Assets returns the relative handler of the embedded browser bundle: native
// JavaScript modules, styles, the notices page, the pinned Leaflet renderer
// and third-party license texts.
//
// Paths are relative to the mount, for example "/ui.css" or
// "/licenses/leaflet-LICENSE.txt". Only allowlisted embedded files are
// served; directories, unknown paths and traversal attempts get 404. Only
// GET and HEAD are accepted. The handler needs no Node runtime, CDN, or
// network access.
func Assets() http.Handler {
	return http.HandlerFunc(serveAsset)
}

func serveAsset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	data, ok := assets.Lookup(name)
	if !ok || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	contentType, ok := contentTypes[path.Ext(name)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	header := w.Header()
	header.Set("Content-Type", contentType)
	header.Set("Cache-Control", "no-cache")
	header.Set("X-Content-Type-Options", "nosniff")
	if path.Ext(name) == ".html" {
		header.Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; base-uri 'none'")
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data)
}
