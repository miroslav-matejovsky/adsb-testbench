package testbench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/ui"
)

// Status is the bounded JSON body of the status route. It is computed from
// local state only: reading it never contacts a remote source and never
// advances virtual time.
//
// RunID is the simulator run this application serves, or for a
// display-only application the run of its last successful refresh; it is
// null when unknown. Display is the display backend's last refresh result,
// null for a simulator-only application. In a display-only application it
// describes upstream availability, independently of local readiness.
type Status struct {
	Mode    Mode           `json:"mode"`
	State   string         `json:"state"`
	RunID   *string        `json:"runId"`
	Display *DisplayStatus `json:"display"`
}

// DisplayStatus summarizes the display backend's last refresh.
type DisplayStatus struct {
	Status        display.Status         `json:"status"`
	LastUpdatedAt *string                `json:"lastUpdatedAt"`
	Error         *display.SourceFailure `json:"error"`
}

// Status returns the current local status.
func (a *App) Status() Status {
	status := Status{Mode: a.mode, State: a.lifecycleState()}
	if a.api != nil {
		runID := a.api.RunID()
		status.RunID = &runID
	}
	if a.display != nil {
		snapshot := a.display.Snapshot()
		status.Display = &DisplayStatus{Status: snapshot.Status, LastUpdatedAt: snapshot.LastUpdatedAt, Error: snapshot.Error}
		if status.RunID == nil && snapshot.Observations != nil {
			runID := snapshot.Observations.RunID
			status.RunID = &runID
		}
	}
	return status
}

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ADS-B test bench</title>
<link rel="stylesheet" href="{{.Assets}}ui.css">
</head>
<body class="tb-page">
<header class="tb-page-header"><h1>ADS-B test bench</h1></header>
<main class="tb-page-main">
<ul>
{{if .Manager}}<li><a href="{{.Manager}}">Simulator manager</a></li>{{end}}
{{if .Aircraft}}<li><a href="{{.Aircraft}}">Received aircraft</a></li>{{end}}
<li><a href="{{.Status}}">Status (JSON)</a></li>
<li><a href="{{.Assets}}notices.html">Third-party notices</a></li>
</ul>
</main>
</body>
</html>
`))

// routes assembles the relative route table of one mode. Routes of other
// modes are 404. A missing trailing slash on a page route is redirected to
// the canonical public path below the configured mount; API and asset
// prefixes are never redirected.
func (a *App) routes(managerPage, aircraftPage http.Handler) (http.Handler, error) {
	links := struct{ Manager, Aircraft, Status, Assets string }{
		Status: a.paths.base + "status", Assets: a.paths.assets,
	}
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets", ui.Assets()))
	mux.Handle("/assets", http.NotFoundHandler())
	mux.HandleFunc("/status", a.serveStatus)

	if managerPage != nil {
		links.Manager = a.paths.base + "manager/"
		mux.Handle("/manager/", http.StripPrefix("/manager", managerPage))
		mux.Handle("/manager", a.redirect(links.Manager))
		mux.Handle("/api/simulator/", http.StripPrefix("/api/simulator", a.api.Handler()))
		mux.Handle("/api/simulator", http.NotFoundHandler())
	}
	if aircraftPage != nil {
		links.Aircraft = a.paths.base + "aircraft/"
		mux.Handle("/aircraft/", http.StripPrefix("/aircraft", aircraftPage))
		mux.Handle("/aircraft", a.redirect(links.Aircraft))
		mux.Handle("/api/display/", http.StripPrefix("/api/display", a.display.Handler()))
		mux.Handle("/api/display", http.NotFoundHandler())
	}

	var index bytes.Buffer
	if err := indexTemplate.Execute(&index, links); err != nil {
		return nil, fmt.Errorf("render index page: %w", err)
	}
	page := index.Bytes()
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; base-uri 'none'")
		if r.Method == http.MethodGet {
			_, _ = w.Write(page)
		}
	})
	return mux, nil
}

// redirect sends GET and HEAD to the canonical page URL below the public
// mount. Other methods are rejected rather than redirected.
func (a *App) redirect(target string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
}

func (a *App) serveStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.RawQuery != "" {
		http.Error(w, "the status route accepts no query", http.StatusBadRequest)
		return
	}
	body, err := json.Marshal(a.Status())
	if err != nil {
		http.Error(w, "the status could not be encoded", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}
