// Unit tests for browser configuration validation. The URL vectors mirror
// internal/urlpath so Go and browser code accept and reject the same bases.
import assert from "node:assert/strict";
import { test } from "node:test";

import {
  ConfigError, resolveBase, validateAircraftConfig, validateManagerConfig, validateStationIds,
} from "../../internal/assets/static/config.js";

const origin = "http://127.0.0.1:8080";

function manager(overrides = {}) {
  return {
    apiBaseUrl: "/bench/a/api/simulator/", assetBaseUrl: "/bench/a/assets/",
    pollIntervalMilliseconds: 1000, requestTimeoutMilliseconds: 5000,
    maxResponseBytes: 1048576, resumeSpeedHundredths: 100, ...overrides,
  };
}

function aircraft(overrides = {}) {
  return {
    apiBaseUrl: "/api/display/", assetBaseUrl: "/assets/",
    pollIntervalMilliseconds: 1000, requestTimeoutMilliseconds: 5000, maxResponseBytes: 1048576,
    stationIds: ["alpha"], freshForNanoseconds: "10000000000", lostAfterNanoseconds: "60000000000",
    historyPageSize: 3, maxHistoryRecords: 6, initialLatitudeDegrees: 50, initialLongitudeDegrees: 14,
    initialZoom: 7, tiles: null, ...overrides,
  };
}

const tiles = {
  urlTemplate: "https://tiles.example.test/{z}/{x}/{y}.png", attributionText: "Example",
  attributionUrl: "https://tiles.example.test/about", minZoom: 0, maxZoom: 18,
};

test("resolveBase keeps nested prefixes and adds one trailing slash", () => {
  assert.equal(resolveBase("k", "/", origin), `${origin}/`);
  assert.equal(resolveBase("k", "/bench/a/api/simulator", origin), `${origin}/bench/a/api/simulator/`);
  assert.equal(resolveBase("k", `${origin}/bench/a/assets`, origin), `${origin}/bench/a/assets/`);
  assert.equal(resolveBase("k", "/bench%20a/", origin), `${origin}/bench%20a/`);
});

test("resolveBase rejects ambiguous and cross-origin bases before any request", () => {
  for (const raw of [
    "", "bench/a/", "//evil.test/api/", "/bench//a/", "/bench/./a/", "/bench/../a/", "/bench/%2e%2E/a/",
    "/bench%2Fa/", "/bench%5Ca/", "/bench\\a/", "/\\evil.test/", "/bench/?a=1", "/bench/?", "/bench/#",
    "/bench\n/", "/bench%0A/", "/bench%zz/", "javascript:alert(1)", "data:text/html,x",
    "http://evil.test/api/", "https://127.0.0.1:8080/api/", "http://127.0.0.1:8081/api/",
    "http://user@127.0.0.1:8080/api/", "http://127.0.0.1:8080/a/../api/",
  ]) {
    assert.throws(() => resolveBase("apiBaseUrl", raw, origin), ConfigError, JSON.stringify(raw));
  }
});

test("validateManagerConfig resolves bases and requires every key", () => {
  const config = validateManagerConfig(manager(), origin);
  assert.equal(config.apiBaseUrl, `${origin}/bench/a/api/simulator/`);
  assert.equal(config.resumeSpeedHundredths, 100);
  assert.ok(Object.isFrozen(config));

  for (const key of Object.keys(manager())) {
    const missing = manager();
    delete missing[key];
    assert.throws(() => validateManagerConfig(missing, origin), ConfigError, `missing ${key}`);
    assert.throws(() => validateManagerConfig(manager({ [key]: null }), origin), ConfigError, `null ${key}`);
  }
  assert.throws(() => validateManagerConfig(manager({ extra: 1 }), origin), ConfigError);
  assert.throws(() => validateManagerConfig(manager({ pollIntervalMilliseconds: 0 }), origin), ConfigError);
  assert.throws(() => validateManagerConfig(manager({ requestTimeoutMilliseconds: 1.5 }), origin), ConfigError);
  assert.throws(() => validateManagerConfig(manager({ maxResponseBytes: 2047 }), origin), ConfigError);
  assert.throws(() => validateManagerConfig(manager({ resumeSpeedHundredths: 0 }), origin), ConfigError);
  assert.throws(() => validateManagerConfig(null, origin), ConfigError);
});

test("validateAircraftConfig parses exact thresholds and nullable tiles", () => {
  const config = validateAircraftConfig(aircraft(), origin);
  assert.equal(config.freshForNanoseconds, 10000000000n);
  assert.equal(config.lostAfterNanoseconds, 60000000000n);
  assert.equal(config.tiles, null);
  assert.deepEqual([...config.stationIds], ["alpha"]);

  const withTiles = validateAircraftConfig(aircraft({ tiles }), origin);
  assert.equal(withTiles.tiles.urlTemplate, tiles.urlTemplate);
  const local = validateAircraftConfig(aircraft({ tiles: { ...tiles, urlTemplate: "/tiles/{z}/{x}/{y}.png" } }), origin);
  assert.equal(local.tiles.urlTemplate, "/tiles/{z}/{x}/{y}.png");
  assert.deepEqual([...validateAircraftConfig(aircraft({ stationIds: [] }), origin).stationIds], []);
});

test("validateAircraftConfig rejects out-of-domain settings", () => {
  const invalid = [
    { stationIds: null }, { stationIds: "alpha" }, { stationIds: ["a", "a"] }, { stationIds: ["a b"] },
    { freshForNanoseconds: 10 }, { freshForNanoseconds: "0" }, { lostAfterNanoseconds: "10000000000" },
    { freshForNanoseconds: "010" }, { historyPageSize: 0 }, { historyPageSize: 1001 },
    { maxHistoryRecords: 2 }, { initialLatitudeDegrees: 91 }, { initialLatitudeDegrees: Number.NaN },
    { initialLongitudeDegrees: -181 }, { initialZoom: 7.5 }, { initialZoom: 25 },
    { tiles: { ...tiles, urlTemplate: "https://t.test/{x}/{y}.png" } },
    { tiles: { ...tiles, urlTemplate: "https://{s}.t.test/{z}/{x}/{y}.png" } },
    { tiles: { ...tiles, urlTemplate: "javascript:alert('{z}{x}{y}')" } },
    { tiles: { ...tiles, urlTemplate: "//t.test/{z}/{x}/{y}.png" } },
    { tiles: { ...tiles, urlTemplate: "https://u@t.test/{z}/{x}/{y}.png" } },
    { tiles: { ...tiles, urlTemplate: "https://t.test/{z}/{x}/{y}.png#a" } },
    { tiles: { ...tiles, attributionText: "" } },
    { tiles: { ...tiles, attributionUrl: "javascript:alert(1)" } },
    { tiles: { ...tiles, minZoom: 8 } },
    { tiles: { ...tiles, extra: true } },
    { tiles: { urlTemplate: tiles.urlTemplate } },
  ];
  for (const overrides of invalid) {
    assert.throws(() => validateAircraftConfig(aircraft(overrides), origin), ConfigError, JSON.stringify(overrides));
  }
});

test("validateStationIds enforces the selection bound", () => {
  assert.throws(() => validateStationIds(["a", "b", "c", "d", "e", "f", "g", "h", "i"]), ConfigError);
  assert.deepEqual([...validateStationIds(["a-1", "B_2"])], ["a-1", "B_2"]);
});
