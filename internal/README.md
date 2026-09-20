# Internal support

Internal support for ADS-B encoding and decoding, serialized simulation pacing,
HTTP lifecycle, command validation, and URL validation. Each package has a
`doc.go` describing its responsibility.

Two packages are test support only and are imported solely by `_test.go`
files: `processtest` runs command processes, and `qualitytest` loads the
shared fixture corpus in [testdata/quality](../testdata/quality/README.md).
