# Display test fixtures

## Contents

This directory holds attribution and semantics for the display test corpus.
The display tests use two fixture classes.

- Synthetic encoded fixtures are built in `fixtures_test.go` rather than
  stored as files. Every frame is encoded by `internal/adsb` from stated wire
  values, so a fixture documents its own input. These frames pin display
  behavior; they are not independent protocol evidence, because they come
  from this project's own encoder.
- Literal reference fixtures come from the shared corpus in
  [testdata/quality](../../testdata/quality/README.md). `reference_test.go`
  decodes its literal raw snapshot of five published frames with their
  original addresses, and `source_test.go` serves the same bytes through an
  in-process provider and an HTTP transport. Expected observations are stored
  data, never output of this decoder.

## Fixture semantics

- Frames carry wire values only. A fixture never asserts simulator truth as
  the expected decoded value; synthetic position fixtures compare against the
  encoded coordinate with a tolerance covering CPR quantization.
- Timestamps are virtual instants. Expiry and CPR pairing fixtures state their
  own instants, so no test depends on a real clock.
- Receiver records keep the historical station settings and revision in effect
  at reception. A fixture that edits a station adds a new revision instead of
  rewriting an existing record.
- Independent expected values exist only for the literal reference fixtures.
  They come from the published examples listed in the corpus README and the
  codec provenance table in `internal/adsb/testdata`.

## Adding a fixture

State the wire values explicitly, give the expected decoded value with its
tolerance, and say which behavior the fixture pins. Fixtures that only repeat
an existing case add maintenance cost without adding evidence.
