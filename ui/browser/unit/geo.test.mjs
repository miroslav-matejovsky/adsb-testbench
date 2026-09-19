// Unit tests for map geometry helpers.
import assert from "node:assert/strict";
import { test } from "node:test";

import {
  clippedByProjection, coverageMetres, METRES_PER_NAUTICAL_MILE, normalizeLongitude,
} from "../../internal/assets/static/geo.js";

test("coverage radii convert with the exact nautical mile", () => {
  assert.equal(METRES_PER_NAUTICAL_MILE, 1852);
  assert.equal(coverageMetres(0), 0);
  assert.equal(coverageMetres(1), 1852);
  assert.equal(coverageMetres(78.5), 145382);
});

test("longitudes normalize consistently across the date line", () => {
  assert.equal(normalizeLongitude(180), -180);
  assert.equal(normalizeLongitude(-180), -180);
  assert.equal(normalizeLongitude(179.5), 179.5);
  assert.equal(normalizeLongitude(-179.5), -179.5);
  assert.equal(normalizeLongitude(0), 0);
  assert.equal(normalizeLongitude(-0), 0);
  assert.equal(normalizeLongitude(540), -180);
});

test("polar latitudes are flagged as clipped by the projection", () => {
  assert.equal(clippedByProjection(85), false);
  assert.equal(clippedByProjection(85.06), true);
  assert.equal(clippedByProjection(-90), true);
});
