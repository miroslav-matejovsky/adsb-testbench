// Unit tests for the DOM-free received-track model.
import assert from "node:assert/strict";
import { test } from "node:test";

import { buildView, normalizeSelection, trackStatus, validateObservations } from "../../internal/assets/static/tracks.js";
import { aircraft, at, isoAt } from "../fixtures.mjs";

const thresholds = { freshForNanoseconds: 10_000_000_000n, lostAfterNanoseconds: 60_000_000_000n };

function snapshot(nowSeconds, list, stationIds = ["alpha"]) {
  return {
    runId: "run-1", now: isoAt(at(nowSeconds)), stationIds, retention: [], expiry: {}, aircraft: list,
  };
}

test("track thresholds are exact at the boundaries", () => {
  const fresh = 10_000_000_000n;
  const lost = 60_000_000_000n;
  assert.equal(trackStatus(0n, fresh, lost), "fresh");
  assert.equal(trackStatus(fresh, fresh, lost), "fresh");
  assert.equal(trackStatus(fresh + 1n, fresh, lost), "stale");
  assert.equal(trackStatus(lost, fresh, lost), "stale");
  assert.equal(trackStatus(lost + 1n, fresh, lost), "lost");
});

test("partial tracks keep only received fields with exact ages", () => {
  const identityOnly = aircraft({
    icao: "ABC001", lastReceivedAt: isoAt(at(9)), identity: { callsign: "TB0001", observedAt: isoAt(at(9)) },
  });
  const altitudeOnly = aircraft({
    icao: "ABC002", lastReceivedAt: isoAt(at(4.5)), altitude: { feet: 0, observedAt: isoAt(at(4.5)) },
  });
  const velocityOnly = aircraft({
    icao: "ABC003", lastReceivedAt: isoAt(at(10)),
    velocity: { observedAt: isoAt(at(10)), values: { groundSpeedKnots: 0, trackDegrees: 0 } },
  });
  const view = buildView(snapshot(10, [velocityOnly, identityOnly, altitudeOnly]), null, thresholds);
  assert.deepEqual(view.rows.map((row) => row.icao), ["ABC001", "ABC002", "ABC003"]);
  const [identity, altitude, velocity] = view.rows;
  assert.equal(identity.identity.ageNs, 1_000_000_000n);
  assert.equal(identity.position, null);
  assert.equal(identity.altitude, null);
  assert.equal(altitude.altitude.feet, 0);
  assert.equal(altitude.altitude.ageNs, 5_500_000_000n);
  assert.equal(velocity.velocity.groundSpeedKnots, 0);
  assert.equal(velocity.velocity.trackDegrees, 0);
  assert.equal(velocity.velocity.ageNs, 0n);
  assert.equal(velocity.identity, null);
  assert.deepEqual(velocity.receivers, ["alpha"]);
});

test("a position without both CPR halves in evidence is rejected", () => {
  const broken = aircraft({
    icao: "ABC001", lastReceivedAt: isoAt(at(1)),
    position: { latitudeDegrees: 50, longitudeDegrees: 14, observedAt: isoAt(at(1)) },
  });
  broken.position.evidence = [];
  assert.throws(() => buildView(snapshot(2, [broken]), null, thresholds), TypeError);
});

test("an aircraft that disappears becomes a tombstone for exactly one view", () => {
  const seen = aircraft({
    icao: "ABC001", lastReceivedAt: isoAt(at(1)), identity: { callsign: "TB0001", observedAt: isoAt(at(1)) },
    position: { latitudeDegrees: 50, longitudeDegrees: 14, observedAt: isoAt(at(1)) },
  });
  const first = buildView(snapshot(2, [seen]), null, thresholds);
  const second = buildView(snapshot(3, []), first, thresholds);
  assert.equal(second.rows.length, 1);
  const tombstone = second.rows[0];
  assert.equal(tombstone.tombstone, true);
  assert.equal(tombstone.status, "lost");
  assert.equal(tombstone.callsign, "TB0001");
  assert.equal(tombstone.position, null, "no old coordinate is carried forward");
  const third = buildView(snapshot(4, []), second, thresholds);
  assert.deepEqual(third.rows, []);
});

test("stale and lost tracks follow the snapshot instant, not wall time", () => {
  const old = aircraft({ icao: "ABC001", lastReceivedAt: isoAt(at(0)), altitude: { feet: 1000, observedAt: isoAt(at(0)) } });
  assert.equal(buildView(snapshot(10, [old]), null, thresholds).rows[0].status, "fresh");
  assert.equal(buildView(snapshot(10.000000001, [old]), null, thresholds).rows[0].status, "stale");
  assert.equal(buildView(snapshot(60, [old]), null, thresholds).rows[0].status, "stale");
  assert.equal(buildView(snapshot(60.000000001, [old]), null, thresholds).rows[0].status, "lost");
});

test("observations must match the exact requested selection and run shape", () => {
  const valid = snapshot(1, [], ["alpha", "bravo"]);
  validateObservations(valid, ["bravo", "alpha"]);
  assert.throws(() => validateObservations(valid, ["alpha"]), /different station selection/);
  assert.throws(() => validateObservations({ ...valid, runId: "" }, ["alpha", "bravo"]), /runId/);
  assert.throws(() => validateObservations({ ...valid, now: "later" }, ["alpha", "bravo"]), TypeError);
  assert.throws(() => validateObservations(null, []), TypeError);
  validateObservations(snapshot(1, [], []), []);
  assert.deepEqual(normalizeSelection(["b", "a", "b"]), ["a", "b"]);
});

test("fields observed after the snapshot instant are rejected", () => {
  const future = aircraft({ icao: "ABC001", lastReceivedAt: isoAt(at(5)), altitude: { feet: 1, observedAt: isoAt(at(5)) } });
  assert.throws(() => buildView(snapshot(4, [future]), null, thresholds), TypeError);
});
