package ui

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/urlpath"
	"github.com/miroslav-matejovsky/adsb-testbench/ui/internal/assets"
)

// pageData is what a page template receives.
type pageData struct {
	AssetBase string
	Config    any
}

// NewManager validates config and returns the manager page handler.
//
// The handler serves one HTML page at its relative root ("/") for GET and
// HEAD. The page loads styles and modules from config.AssetBaseURL and mounts
// a manager component that talks to config.APIBaseURL. The page is rendered
// once here, so a configuration or template error is returned before any
// request is served. The handler starts no goroutine.
func NewManager(config ManagerConfig) (http.Handler, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("create manager page: %w", err)
	}
	api, asset := normalizedBases(config.APIBaseURL, config.AssetBaseURL)
	return newPage("manager.html", asset, config.browser(api, asset), "")
}

// NewAircraftDisplay validates config and returns the aircraft display page
// handler. It behaves like NewManager; config.APIBaseURL addresses the
// display backend, never the simulator.
func NewAircraftDisplay(config AircraftDisplayConfig) (http.Handler, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("create aircraft display page: %w", err)
	}
	api, asset := normalizedBases(config.APIBaseURL, config.AssetBaseURL)
	tileOrigin := ""
	if config.Tiles != nil {
		tileOrigin = originOf(config.Tiles.URLTemplate)
	}
	return newPage("aircraft.html", asset, config.browser(api, asset), tileOrigin)
}

// normalizedBases returns both validated bases with one trailing slash.
func normalizedBases(api, asset string) (string, string) {
	normalizedAPI, _ := urlpath.ParseBrowserBase(api)
	normalizedAsset, _ := urlpath.ParseBrowserBase(asset)
	return normalizedAPI, normalizedAsset
}

// originOf returns the scheme and host of an absolute tile template, or ""
// for a root-relative template.
func originOf(template string) string {
	parsed, err := url.Parse(strings.NewReplacer("{z}", "0", "{x}", "0", "{y}", "0").Replace(template))
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// page is one pre-rendered HTML document.
type page struct {
	body   []byte
	policy string
}

func newPage(name, assetBase string, config any, tileOrigin string) (*page, error) {
	text, err := assets.Template(name)
	if err != nil {
		return nil, fmt.Errorf("read template %s: %w", name, err)
	}
	parsed, err := template.New(name).Parse(text)
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", name, err)
	}
	var body bytes.Buffer
	if err := parsed.Execute(&body, pageData{AssetBase: assetBase, Config: config}); err != nil {
		return nil, fmt.Errorf("render template %s: %w", name, err)
	}
	images := "'self' data:"
	if tileOrigin != "" {
		images += " " + tileOrigin
	}
	return &page{
		body: body.Bytes(),
		policy: "default-src 'none'; script-src 'self'; style-src 'self'; img-src " + images +
			"; connect-src 'self'; base-uri 'none'; form-action 'none'",
	}, nil
}

// ServeHTTP serves the page at the relative root only.
func (p *page) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Content-Security-Policy", p.policy)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(p.body)
}
