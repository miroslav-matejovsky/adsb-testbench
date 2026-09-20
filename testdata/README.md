# Repository test data

Test inputs shared by several packages. Package-local fixtures stay beside
their package in its own `testdata` directory.

| Directory | Contents |
| --- | --- |
| [quality](quality/README.md) | Attributed reference frames, engine scenarios, and a literal display reference snapshot, loaded by `internal/qualitytest` |

Nothing here is read by production code. Tests read it through explicit
relative paths and never download or regenerate it.
