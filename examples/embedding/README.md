# External embedding example

A separate Go module that embeds the test bench exactly as an external host
would. It imports only public packages (`testbench`, `simulator`,
`simulation`, `display`, `ui`); a `replace` directive points it at this
checkout, so it always compiles against the current code.

It demonstrates:

- a host-owned `http.Server`, logger, interrupt handling and shutdown order
  (drain HTTP, then cancel and join the benches);
- two independent combined benches mounted below `/bench/a/` and
  `/bench/b/`, each with its own fresh run ID, stations and lifecycle;
- a custom page at `/custom/` that lays out its own HTML and mounts a manager
  from bench A and an aircraft display from bench B through
  `components.js` with explicit API and asset URLs;
- complete explicit configuration in `benchConfig`; nothing relies on a
  package default.

Run the checks from the repository root:

```text
go -C examples/embedding vet ./...
go -C examples/embedding test ./...
```

Run the host (the address is required):

```text
go -C examples/embedding run . -listen 127.0.0.1:18480
```

Then open `http://127.0.0.1:18480/bench/a/`, `/bench/b/` or `/custom/`.
