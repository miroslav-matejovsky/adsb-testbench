// Browser-side configuration validation.
//
// Mirrors the Go validators in ui/config.go. A component validates its
// complete configuration and resolves its URL bases against the page origin
// before it sends any request. No parser supplies a missing field; only
// tiles may be null.

import { parseDecimal } from "./time.js";

/** Bounds shared with ui/config.go. */
export const limits = Object.freeze({
  maxSelectedStations: 8,
  maxHistoryPageSize: 1000,
  maxHistoryRecords: 100000,
  maxZoom: 24,
  minResponseBytes: 2048,
});

const stationIdPattern = /^[A-Za-z0-9_-]{1,64}$/;
// eslint-disable-next-line no-control-regex
const controlPattern = /[\u0000-\u001f\u007f-\u009f]/;

/** Thrown for any configuration outside its accepted domain. */
export class ConfigError extends Error {
  constructor(message) {
    super(`invalid UI configuration: ${message}`);
    this.name = "ConfigError";
  }
}

function requireKeys(config, keys, nullable = []) {
  if (config === null || typeof config !== "object" || Array.isArray(config)) {
    throw new ConfigError("configuration must be an object");
  }
  for (const key of Object.keys(config)) {
    if (!keys.includes(key)) {
      throw new ConfigError(`unknown key ${JSON.stringify(key)}`);
    }
  }
  for (const key of keys) {
    if (!(key in config) || config[key] === undefined) {
      throw new ConfigError(`missing key ${JSON.stringify(key)}`);
    }
    if (config[key] === null && !nullable.includes(key)) {
      throw new ConfigError(`${key} must not be null`);
    }
  }
}

function positiveInteger(config, key) {
  const value = config[key];
  if (!Number.isSafeInteger(value) || value <= 0) {
    throw new ConfigError(`${key} must be a positive integer`);
  }
  return value;
}

function integerWithin(config, key, lo, hi) {
  const value = config[key];
  if (!Number.isSafeInteger(value) || value < lo || value > hi) {
    throw new ConfigError(`${key} must be an integer within [${lo},${hi}]`);
  }
  return value;
}

function finiteWithin(config, key, lo, hi) {
  const value = config[key];
  if (typeof value !== "number" || !Number.isFinite(value) || value < lo || value > hi) {
    throw new ConfigError(`${key} must be a finite number within [${lo},${hi}]`);
  }
  return value;
}

function hasBadPathSyntax(text) {
  if (controlPattern.test(text) || text.includes("\\") || text.includes("?") || text.includes("#")) {
    return true;
  }
  // Inspect the literal path text: URL parsing would already have removed
  // the dot segments and separators this check exists to reject.
  const path = text.startsWith("/") ? text : text.replace(/^https?:\/\/[^/]*/i, "");
  if (path.includes("//")) {
    return true;
  }
  for (const segment of path.split("/")) {
    let decoded;
    try {
      decoded = decodeURIComponent(segment);
    } catch {
      return true;
    }
    if (decoded === "." || decoded === ".." || (decoded !== segment && /[/\\]/.test(decoded)) ||
        controlPattern.test(decoded)) {
      return true;
    }
  }
  return false;
}

/**
 * Resolves one API or asset base against the page origin and returns its
 * absolute href with one trailing slash. The base must be a root-relative
 * path or an absolute http(s) URL on the page's own origin.
 */
export function resolveBase(key, raw, pageOrigin) {
  if (typeof raw !== "string" || raw === "") {
    throw new ConfigError(`${key} must be a nonempty string`);
  }
  const rootRelative = raw.startsWith("/") && !raw.startsWith("//");
  if (!rootRelative && !/^https?:\/\//i.test(raw)) {
    throw new ConfigError(`${key} must be a root-relative path or an absolute http(s) URL`);
  }
  let url;
  try {
    if (hasBadPathSyntax(raw)) {
      throw new ConfigError(`${key} contains ambiguous path syntax`);
    }
    url = new URL(raw, pageOrigin);
  } catch (error) {
    throw error instanceof ConfigError ? error : new ConfigError(`${key} is not a valid URL`);
  }
  if (url.username !== "" || url.password !== "") {
    throw new ConfigError(`${key} must not carry userinfo`);
  }
  if (url.origin !== pageOrigin) {
    throw new ConfigError(`${key} resolves to ${url.origin}, not the page origin ${pageOrigin}`);
  }
  return url.href.endsWith("/") ? url.href : url.href + "/";
}

function commonConfig(config, pageOrigin) {
  const maxResponseBytes = config.maxResponseBytes;
  if (!Number.isSafeInteger(maxResponseBytes) || maxResponseBytes < limits.minResponseBytes) {
    throw new ConfigError(`maxResponseBytes must be an integer of at least ${limits.minResponseBytes}`);
  }
  return {
    apiBaseUrl: resolveBase("apiBaseUrl", config.apiBaseUrl, pageOrigin),
    assetBaseUrl: resolveBase("assetBaseUrl", config.assetBaseUrl, pageOrigin),
    pollIntervalMilliseconds: positiveInteger(config, "pollIntervalMilliseconds"),
    requestTimeoutMilliseconds: positiveInteger(config, "requestTimeoutMilliseconds"),
    maxResponseBytes,
  };
}

const managerKeys = [
  "apiBaseUrl", "assetBaseUrl", "pollIntervalMilliseconds", "requestTimeoutMilliseconds",
  "maxResponseBytes", "resumeSpeedHundredths",
];

/** Validates a manager configuration and returns a frozen resolved copy. */
export function validateManagerConfig(config, pageOrigin) {
  requireKeys(config, managerKeys);
  return Object.freeze({
    ...commonConfig(config, pageOrigin),
    resumeSpeedHundredths: positiveInteger(config, "resumeSpeedHundredths"),
  });
}

const aircraftKeys = [
  "apiBaseUrl", "assetBaseUrl", "pollIntervalMilliseconds", "requestTimeoutMilliseconds",
  "maxResponseBytes", "stationIds", "freshForNanoseconds", "lostAfterNanoseconds",
  "historyPageSize", "maxHistoryRecords", "initialLatitudeDegrees", "initialLongitudeDegrees",
  "initialZoom", "tiles",
];

const tileKeys = ["urlTemplate", "attributionText", "attributionUrl", "minZoom", "maxZoom"];

/** Validates a station selection: an array of unique valid IDs. */
export function validateStationIds(stationIds) {
  if (!Array.isArray(stationIds)) {
    throw new ConfigError("stationIds must be an array, including when empty");
  }
  if (stationIds.length > limits.maxSelectedStations) {
    throw new ConfigError(`at most ${limits.maxSelectedStations} stations may be selected`);
  }
  const seen = new Set();
  for (const id of stationIds) {
    if (typeof id !== "string" || !stationIdPattern.test(id)) {
      throw new ConfigError(`station ID ${JSON.stringify(id)} is invalid`);
    }
    if (seen.has(id)) {
      throw new ConfigError(`station ${JSON.stringify(id)} is selected twice`);
    }
    seen.add(id);
  }
  return Object.freeze([...stationIds]);
}

/** Substitutes numeric placeholders into a tile URL template. */
export function tileUrl(template, z, x, y) {
  return template.replace("{z}", String(z)).replace("{x}", String(x)).replace("{y}", String(y));
}

function validateTiles(tiles, pageOrigin) {
  requireKeys(tiles, tileKeys);
  const template = tiles.urlTemplate;
  if (typeof template !== "string") {
    throw new ConfigError("tiles.urlTemplate must be a string");
  }
  for (const placeholder of ["{z}", "{x}", "{y}"]) {
    if (template.split(placeholder).length !== 2) {
      throw new ConfigError(`tiles.urlTemplate must contain ${placeholder} exactly once`);
    }
  }
  const sample = tileUrl(template, 0, 0, 0);
  if (/[{}]/.test(sample) || controlPattern.test(sample) || /[\\#]/.test(sample)) {
    throw new ConfigError("tiles.urlTemplate contains an unsupported placeholder, control character, backslash or fragment");
  }
  const rootRelative = sample.startsWith("/") && !sample.startsWith("//");
  if (!rootRelative) {
    let url;
    try {
      url = new URL(sample);
    } catch {
      throw new ConfigError("tiles.urlTemplate is not a valid URL");
    }
    if (!["http:", "https:"].includes(url.protocol) || url.username !== "" || url.password !== "") {
      throw new ConfigError("tiles.urlTemplate must be root-relative or an absolute http(s) URL without userinfo");
    }
  } else {
    new URL(sample, pageOrigin);
  }
  if (typeof tiles.attributionText !== "string" || tiles.attributionText.trim() === "" ||
      controlPattern.test(tiles.attributionText)) {
    throw new ConfigError("tiles.attributionText must be nonempty text");
  }
  let link;
  try {
    link = new URL(tiles.attributionUrl);
  } catch {
    throw new ConfigError("tiles.attributionUrl is not a valid URL");
  }
  if (!["http:", "https:"].includes(link.protocol) || link.username !== "" || link.password !== "" ||
      controlPattern.test(tiles.attributionUrl) || tiles.attributionUrl.includes("\\")) {
    throw new ConfigError("tiles.attributionUrl must be an absolute http(s) URL without userinfo");
  }
  const minZoom = integerWithin(tiles, "minZoom", 0, limits.maxZoom);
  const maxZoom = integerWithin(tiles, "maxZoom", minZoom, limits.maxZoom);
  return Object.freeze({
    urlTemplate: template, attributionText: tiles.attributionText,
    attributionUrl: link.href, minZoom, maxZoom,
  });
}

/** Validates an aircraft display configuration and returns a resolved copy. */
export function validateAircraftConfig(config, pageOrigin) {
  requireKeys(config, aircraftKeys, ["tiles"]);
  const common = commonConfig(config, pageOrigin);
  let freshFor;
  let lostAfter;
  try {
    freshFor = parseDecimal(config.freshForNanoseconds);
    lostAfter = parseDecimal(config.lostAfterNanoseconds);
  } catch {
    throw new ConfigError("freshForNanoseconds and lostAfterNanoseconds must be canonical decimal strings");
  }
  if (freshFor <= 0n || lostAfter <= freshFor) {
    throw new ConfigError("track thresholds must satisfy 0 < freshFor < lostAfter");
  }
  const historyPageSize = integerWithin(config, "historyPageSize", 1, limits.maxHistoryPageSize);
  const maxHistoryRecords = integerWithin(config, "maxHistoryRecords", historyPageSize, limits.maxHistoryRecords);
  const tiles = config.tiles === null ? null : validateTiles(config.tiles, pageOrigin);
  const initialZoom = integerWithin(config, "initialZoom",
    tiles ? tiles.minZoom : 0, tiles ? tiles.maxZoom : limits.maxZoom);
  return Object.freeze({
    ...common,
    stationIds: validateStationIds(config.stationIds),
    freshForNanoseconds: freshFor,
    lostAfterNanoseconds: lostAfter,
    historyPageSize,
    maxHistoryRecords,
    initialLatitudeDegrees: finiteWithin(config, "initialLatitudeDegrees", -90, 90),
    initialLongitudeDegrees: finiteWithin(config, "initialLongitudeDegrees", -180, 180),
    initialZoom,
    tiles,
  });
}
