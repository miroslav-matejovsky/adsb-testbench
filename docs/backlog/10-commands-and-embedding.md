---
value: "High"
effort: "L"
complexity: "medium"
dependencies: ["07-display-backend.md","08-manager-ui.md","09-aircraft-map-and-inspector.md"]
---

# Combined commands and host embedding

## Area

cmd/adsb-testbench, cmd/simulator, cmd/display, testbench, internal/cli, internal/urlpath, and internal/httpserver.

## Work

Implement combined and standalone commands with explicit documented configuration, process-owned logging, signals, and graceful shutdown. Compose a combined bench with an in-process source. Mount public handlers and reusable UI components below configured prefixes; let hosts own servers and lifecycle. Validate addresses and URLs early. Enable runnable Task commands and command reachability checks when entry points exist.

## Acceptance criteria

Test combined and separate deployments, prefixed mounts, invalid configuration, startup failures, and graceful shutdown. Compile public examples as an external consumer. Add a browser fixture with two independent benches and component mount/destroy cycles.

## Dependencies

- [07-display-backend](07-display-backend.md)
- [08-manager-ui](08-manager-ui.md)
- [09-aircraft-map-and-inspector](09-aircraft-map-and-inspector.md)
