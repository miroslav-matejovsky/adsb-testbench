# Command configuration

Complete example configurations for the three commands. Every key is
required and every value is an explicit choice; no command or parser fills a
missing value. Files are strict JSON objects: missing, `null`, unknown and
duplicate keys and trailing values are rejected. Each file carries a nonempty
`$comment` string that documents its own settings.

| File | Command | Listen address |
| --- | --- | --- |
| `combined.json` | `adsb-testbench` | `127.0.0.1:18480` |
| `simulator.json` | `simulator` | `127.0.0.1:18481` |
| `display.json` | `display` (reads `simulator.json`'s API) | `127.0.0.1:18482` |

Both command flags are required: `-config PATH` names the file, relative to
the working directory or absolute, and `-config-max-bytes N` bounds its size.

```text
task run-combined CONFIG=configs/combined.json CONFIG_MAX_BYTES=65536
task run-simulator CONFIG=configs/simulator.json CONFIG_MAX_BYTES=65536
task run-display CONFIG=configs/display.json CONFIG_MAX_BYTES=65536
```

## Units

`*Nanoseconds` values are canonical decimal strings of nanoseconds, so large
values stay exact. `*Milliseconds` and `*Bytes` are JSON integers. Angles are
degrees, altitudes pressure-altitude feet, distances metres or nautical
miles as named, speeds knots, and `speedHundredths` is virtual time per real
time in hundredths (100 is real time, 0 is paused).

## Sections

| Section | Combined | Simulator | Display |
| --- | --- | --- | --- |
| `$comment` | required | required | required |
| `server` | required | required | required |
| `logging` | required | required | required |
| `simulator` | required | required | - |
| `simulatorApi` | required | required | - |
| `stations` | required | required | - |
| `display` | required | - | required |
| `managerUi` | required | required | - |
| `aircraftUi` | required | - | required |
| `source` | - | - | required |
| `transport` | - | - | required |

### server

| Key | Meaning | Accepted values |
| --- | --- | --- |
| `listenAddress` | TCP address to bind | explicit `host:port`; IPv4, bracketed IPv6 or host name; port 0-65535 (0 picks a free port); no URL syntax |
| `publicBasePath` | Mount prefix of every route and browser URL | `/` or an absolute path such as `/bench/a/`; no dot segments, empty segments, backslashes, queries or encoded separators |
| `readHeaderTimeoutNanoseconds`, `readTimeoutNanoseconds`, `writeTimeoutNanoseconds`, `idleTimeoutNanoseconds` | HTTP server limits | positive; the write timeout must exceed `simulatorApi` and `display` request timeouts |
| `shutdownTimeoutNanoseconds` | Graceful drain budget before connections are closed | positive |
| `maxHeaderBytes` | Request header bound | positive |

Browser API and asset URLs are derived from `publicBasePath` and fixed route
names (`api/simulator/`, `api/display/`, `assets/`); they are not separate
settings.

### logging

`level` is `debug`, `info`, `warn` or `error`; `format` is `text` or `json`.
Logs go to standard error.

### simulator

The existing simulator document: one `simulation` object with `id`,
`startTime` (UTC RFC3339Nano), `seed` (decimal string), `initialAircraftCount`
(0-100), `speedHundredths` (0-10000) and `spawn` ranges (`min`/`max` for
latitude -85..85, longitude -180..180, altitude -1000..50175 ft, ground speed
0..1000 kt, track 0..360 exclusive, vertical rate -10000..10000 ft/min).

`simulation.id` is a label. Each launch appends a random suffix, so the
effective run ID (logged at startup and served in `/status` and `metadata`)
is fresh even when the same file is reused; commands and history cursors
from an earlier launch are rejected as conflicts. Seed and start time are
used unchanged, so generated traffic is reproducible. The run starts paused
while the configured stations are installed, then the command restores
`speedHundredths` immediately before serving.

### simulatorApi

`maxRequestBytes` (positive), `maxResponseBytes` (at least 2048),
`requestTimeoutNanoseconds` (positive) and `coverageReferenceAltitudeFeet`
(-1000..50175), the pressure altitude every published synthetic coverage
estimate uses.

### stations

An array of complete station settings, possibly empty: `id` (1-64 ASCII
letters, digits, `-` or `_`, unique), `enabled` (`true` or `false`),
`latitudeDegrees` (-90..90), `longitudeDegrees` (-180..180),
`siteElevationMetres` (-500..9000), `antennaHeightMetres` (0..500),
`antennaGainDBi` (-10..40), `sensitivityDBm` (-140..0), `systemLossDB`
(0..30) and `frameLossProbability` (0..1). Zero and `false` are values.

### display

Field lifetimes `identityExpiryNanoseconds`, `positionExpiryNanoseconds`,
`altitudeExpiryNanoseconds`, `velocityExpiryNanoseconds` (positive virtual
durations), and backend bounds `maxRequestBytes`, `maxResponseBytes` (at
least 2048) and `requestTimeoutNanoseconds`.

### managerUi

`pollIntervalMilliseconds`, `requestTimeoutMilliseconds` (positive),
`maxResponseBytes` (at least `simulatorApi.maxResponseBytes`) and
`resumeSpeedHundredths` (positive; used by Resume when no positive speed
was observed in the run).

### aircraftUi

`pollIntervalMilliseconds`, `requestTimeoutMilliseconds`, `maxResponseBytes`
(at least `display.maxResponseBytes`); `stationIds`, the initial selection
(possibly empty; in the combined file it may only name configured stations);
`freshForNanoseconds` and `lostAfterNanoseconds` (track classes, 0 < fresh <
lost); `historyPageSize` (1..1000) and `maxHistoryRecords` (page size..100000);
`initialLatitudeDegrees`, `initialLongitudeDegrees`, `initialZoom` (0..24); and
`tiles`.

`tiles` is `null` to disable map tiles (the examples do this; the map then
makes no tile request). To enable a host-approved provider, supply the
complete object:

```json
{
  "urlTemplate": "https://tiles.example.org/{z}/{x}/{y}.png",
  "attributionText": "Example tiles",
  "attributionUrl": "https://tiles.example.org/copyright",
  "minZoom": 0,
  "maxZoom": 18
}
```

The template needs `{z}`, `{x}` and `{y}` exactly once and no other
placeholder; the attribution is shown as text with a link.

### source

The display command's upstream: `baseUrl` (absolute http(s) URL of a
simulator's `/api/simulator` route, including any mount prefix),
`timeoutNanoseconds` (positive, at most `display.requestTimeoutNanoseconds`),
`maxRequestBytes` and `maxResponseBytes`. An unreachable source is an
operational state reported in `/status` and the page, not a configuration
error.

### transport

The display command's outbound HTTP transport: `dialTimeoutNanoseconds`,
`keepAliveNanoseconds`, `tlsHandshakeTimeoutNanoseconds`,
`responseHeaderTimeoutNanoseconds`, `idleConnTimeoutNanoseconds` (all
positive), and `maxIdleConns`, `maxIdleConnsPerHost`, `maxConnsPerHost` (all
positive). Proxy environment variables are not used.
