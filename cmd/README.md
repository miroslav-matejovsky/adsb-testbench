# Commands

| Command | Package | Serves | Example configuration |
| --- | --- | --- | --- |
| `adsb-testbench` | `cmd/adsb-testbench` | Combined simulator, manager, in-process display and aircraft page | `configs/combined.json` |
| `simulator` | `cmd/simulator` | Simulator API and manager page | `configs/simulator.json` |
| `display` | `cmd/display` | Aircraft page and display API over a remote simulator | `configs/display.json` |

Each `main` only calls `cli.Main` with its mode; flag parsing, configuration
loading, fresh run identity, station installation, serving and shutdown live
in `internal/cli`. Both `-config` and `-config-max-bytes` are required:

```text
go run ./cmd/adsb-testbench -config configs/combined.json -config-max-bytes 65536
go run ./cmd/simulator -config configs/simulator.json -config-max-bytes 65536
go run ./cmd/display -config configs/display.json -config-max-bytes 65536
```

The separate simulator and display examples work together: the display's
`source.baseUrl` points at the simulator's `/api/simulator` route. The
configuration format is documented in [configs/README.md](../configs/README.md).
