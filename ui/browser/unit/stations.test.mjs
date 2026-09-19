// Unit tests for station draft validation.
import assert from "node:assert/strict";
import { test } from "node:test";

import { parseStationDraft, settingsOf, stationFields, stationPath } from "../../internal/assets/static/stations.js";

const complete = {
  id: "alpha_1", enabled: "false", latitudeDegrees: "-90", longitudeDegrees: "180", siteElevationMetres: "0",
  antennaHeightMetres: "0", antennaGainDBi: "-10", sensitivityDBm: "0", systemLossDB: "0", frameLossProbability: "0",
};

test("every field round-trips with explicit zero and false values", () => {
  const { settings, errors } = parseStationDraft(complete);
  assert.equal(errors, undefined);
  assert.deepEqual(settings, {
    id: "alpha_1", enabled: false, latitudeDegrees: -90, longitudeDegrees: 180, siteElevationMetres: 0,
    antennaHeightMetres: 0, antennaGainDBi: -10, sensitivityDBm: 0, systemLossDB: 0, frameLossProbability: 0,
  });
  assert.deepEqual(settingsOf({ ...settings, revision: "1", createdAt: "x" }), settings);
});

test("each numeric field rejects empty, non-finite and out-of-range values", () => {
  for (const field of stationFields) {
    for (const text of ["", " ", "abc", "NaN", "Infinity", "1,5", "0x1", String(field.max + 1), String(field.min - 1)]) {
      const { errors } = parseStationDraft({ ...complete, [field.key]: text });
      assert.ok(errors?.[field.key], `${field.key}=${JSON.stringify(text)}`);
    }
    for (const text of [String(field.min), String(field.max)]) {
      assert.equal(parseStationDraft({ ...complete, [field.key]: text }).errors, undefined, `${field.key}=${text}`);
    }
  }
});

test("identity and reception choice are required explicitly", () => {
  assert.ok(parseStationDraft({ ...complete, enabled: "" }).errors.enabled);
  for (const id of ["", "a b", "a/b", "x".repeat(65), "caf\u00e9"]) {
    assert.ok(parseStationDraft({ ...complete, id }).errors.id, JSON.stringify(id));
  }
});

test("station routes encode the ID as one path segment", () => {
  assert.equal(stationPath("alpha-1_x"), "stations/alpha-1_x");
  assert.equal(stationPath("a/b"), "stations/a%2Fb");
});
