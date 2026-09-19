// Command browserhost is the test-only HTTP host for the Playwright browser
// tests in ui/browser. It lives under testdata, so `go ./...` patterns never
// build, vet, or ship it. Playwright starts it with four loopback listeners
// (see playwright.config.mjs):
//
//	go run ./ui/testdata/browserhost -listen 127.0.0.1:18431 \
//	  -listen-combined 127.0.0.1:18432 -listen-simulator 127.0.0.1:18433 \
//	  -listen-display 127.0.0.1:18434
//
// All listeners are bound before any is served, so /ready answering means
// every origin is reachable.
//
// # Presentation origin (-listen)
//
// Real embedded pages and assets from package ui at the root and below
// /bench/a/, with API routes deliberately absent: presentation tests fulfil
// every API request with controlled responses.
//
//	/ready                     readiness probe polled by Playwright
//	{prefix}manager/           manager page (API base {prefix}api/simulator/)
//	{prefix}aircraft/          aircraft page without tiles (API {prefix}api/display/)
//	{prefix}aircraft-tiles/    aircraft page with local tiles from /tiles/
//	{prefix}assets/            ui.Assets()
//	/harness/                  two empty roots and the public component API
//	/tiles/{z}/{x}/{y}.png     deterministic 1x1 PNG tiles
//
// The same origin also hosts real combined benches for integration tests:
// /int/a/ (deployment), /int/c/ and /int/d/ (independent benches), /int/e/
// (restart), and /int/components/, a host page that mounts public
// components of bench C with explicit URLs.
//
// # Separate and root origins
//
//	-listen-combined    a combined bench at the root "/"
//	-listen-simulator   simulator-only benches at "/" and "/nested/sim/"
//	-listen-display     display-only benches at "/" and "/nested/display/",
//	                    reading the simulator origin over HTTP
//
// # Determinism and trust boundary
//
// Every bench is paused (speed 0) and gets stations alpha and bravo at
// startup, so virtual time never moves on its own; aircraft created by a
// count change emit creation reports the stations receive. The only test
// control is POST /fixture/restart?bench=NAME on the presentation origin,
// which replaces one nested bench with a new run ID. It exists only in this
// fixture: production handlers expose no time-advance, fault-injection,
// restart or process-exit endpoint.
package main
