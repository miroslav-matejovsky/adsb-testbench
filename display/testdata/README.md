# Display test fixtures

## Contents

This directory holds attribution and semantics for the display test corpus.
The corpus itself is built in `fixtures_test.go` rather than stored as files:
every frame is encoded by `internal/adsb` from stated wire values, so a
fixture documents its own input instead of hiding it in an opaque blob.

## Fixture semantics

- Frames carry wire values only. A fixture never asserts simulator truth as
  the expected decoded value; position fixtures compare against the encoded
  coordinate with a tolerance covering CPR quantization.
- Timestamps are virtual instants. Expiry and CPR pairing fixtures state their
  own instants, so no test depends on a real clock.
- Receiver records keep the historical station settings and revision in effect
  at reception. A fixture that edits a station adds a new revision instead of
  rewriting an existing record.
- Independent expected values for identification, altitude, velocity, and CPR
  come from the published references listed in `internal/adsb/doc.go`,
  principally ICAO Doc 9871 as reproduced in MIT Lincoln Laboratory ATC-334
  and the NASA CPR description. They are not derived from this decoder.

## Adding a fixture

State the wire values explicitly, give the expected decoded value with its
tolerance, and say which behavior the fixture pins. Fixtures that only repeat
an existing case add maintenance cost without adding evidence.
