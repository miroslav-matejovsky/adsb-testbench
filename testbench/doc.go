// Package testbench composes complete ADS-B test bench applications from the
// simulator, display and ui packages.
//
// [New] builds a combined application: one simulator and its API, and one
// display that reads the same API in process through
// display.NewInProcessSource and display.NewInProcessStationSource. No
// combined read goes through HTTP. [NewSimulator] builds a simulator-only
// application with the API and manager. [NewDisplay] builds a display-only
// application that reads both received evidence and station discovery over
// HTTP from another application's simulator API; browsers still contact
// only this application.
//
// Every configuration is complete and explicit and is validated before
// anything is created. Constructors start no listener, goroutine, poller or
// signal registration. The simulation ID must be fresh for every App,
// because it identifies one engine lifetime; commands append a random suffix
// to their configured label for that reason.
//
// # Routes
//
// [App.Handler] serves relative routes. A host mounts it below the
// configured PublicBasePath and strips that prefix once:
//
//	mux.Handle("/bench/a/", http.StripPrefix("/bench/a", app.Handler()))
//
//	Relative route     Combined        Simulator       Display
//	/                  links           manager link    aircraft link
//	/manager/          manager page    manager page    404
//	/aircraft/         aircraft page   404             aircraft page
//	/api/simulator/    simulator API   simulator API   404
//	/api/display/      display API     404             display API
//	/assets/           embedded UI     embedded UI     embedded UI
//	/status            local status    local status    local and upstream status
//
// Browser URLs are derived from PublicBasePath and these fixed names, never
// from the request host or proxy headers. "/manager" and "/aircraft" without
// a trailing slash redirect GET and HEAD to the canonical public path; API
// and asset prefixes are never redirected.
//
// # Lifecycle
//
// [App.Run] may be called once. Combined and simulator applications run
// their one simulator until ctx is canceled; a display-only application only
// waits, because the display never polls on its own. Cancellation is a clean
// stop. The host owns listeners, logging, signals and shutdown ordering; see
// internal/httpserver for the drain-then-cancel order the commands use.
// [App.Status] and the /status route report mode, lifecycle state, the known
// run ID and the display's last refresh result from local state only.
package testbench
