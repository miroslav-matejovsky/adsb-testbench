// Checks that browser fixtures consume the shared reference corpus in
// testdata/quality instead of private copies of published frames.
import assert from "node:assert/strict";
import { test } from "node:test";

import { displayReference, frames, referenceCorpus } from "../fixtures.mjs";

test("the shared corpus holds the five published frames", () => {
  assert.deepEqual(referenceCorpus.frames.map((frame) => frame.id), [
    "identification-4840d6",
    "position-40621d-even",
    "position-40621d-odd",
    "velocity-485020-ground",
    "velocity-a05f21-airspeed",
  ]);
  for (const frame of referenceCorpus.frames) {
    assert.equal(frame.classification, "external");
    assert.match(frame.hex, /^[0-9A-F]{28}$/);
    assert.ok(frame.sourceUrl.startsWith("https://"), frame.id);
  }
});

test("presentation fixtures reuse the corpus bytes", () => {
  const hex = Object.fromEntries(referenceCorpus.frames.map((frame) => [frame.id, frame.hex]));
  assert.equal(frames.identification, hex["identification-4840d6"]);
  assert.equal(frames.positionEven, hex["position-40621d-even"]);
  assert.equal(frames.positionOdd, hex["position-40621d-odd"]);
  assert.equal(frames.velocity, hex["velocity-485020-ground"]);
});

test("the display reference keeps original addresses and literal fields", () => {
  const hex = Object.fromEntries(referenceCorpus.frames.map((frame) => [frame.id, frame.hex]));
  assert.deepEqual(displayReference.snapshot.records.map((record) => record.frame),
    displayReference.frames.map((id) => hex[id]));

  const expected = displayReference.expected.aircraft;
  assert.deepEqual(expected.map((entry) => entry.icao), ["40621D", "4840D6", "485020", "A05F21"]);
  const byIcao = Object.fromEntries(expected.map((entry) => [entry.icao, entry]));
  assert.equal(byIcao["4840D6"].identity.callsign, "KLM1023");
  assert.equal(byIcao["4840D6"].position, null);
  assert.equal(byIcao["40621D"].barometricAltitude.feet, 38000);
  assert.equal(byIcao["485020"].velocity.verticalRateFeetPerMinute.value, -832);
  assert.equal(byIcao["A05F21"].velocity.headingDegrees, 243.984375);
  assert.equal(byIcao["A05F21"].velocity.groundSpeedKnots, null);
});
