// Deterministic fake simulator and display APIs for presentation tests.
//
// Each fake is stateful and fulfils the documented relative routes below
// its prefix with the exact wire shapes of package simulatorapi. Virtual time
// only moves when a test calls advance(); browser timers are controlled
// separately with the Playwright clock. Tests can hold a request behind a
// barrier, fail the next matching request, or replace the run. This module
// does not import Playwright, so unit tests reuse its data builders.

/** Fulfils one request with a JSON body and status. */
export function json(route, body, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json; charset=utf-8",
    headers: { "Cache-Control": "no-store" },
    body: typeof body === "string" ? body : JSON.stringify(body),
  });
}

/** A promise with its resolve function, for response barriers. */
export function barrier() {
  let release;
  const promise = new Promise((resolve) => {
    release = resolve;
  });
  return { promise, release };
}

export const frames = Object.freeze({
  identification: "8D4840D6202CC371C32CE0576098",
  positionEven: "8D40621D58C382D690C8AC2863A7",
  positionOdd: "8D40621D58C386435CC412692AD6",
  velocity: "8D485020994409940838175B284F",
});

const BILLION = 1_000_000_000n;
const epoch = Date.UTC(2024, 2, 5, 12, 0, 0);

/** Formats BigInt nanoseconds since the Unix epoch as RFC3339Nano UTC. */
export function isoAt(nanoseconds) {
  const milliseconds = Number(nanoseconds / 1_000_000n);
  const fraction = nanoseconds % BILLION;
  const base = new Date(milliseconds).toISOString().slice(0, 19);
  const digits = fraction === 0n ? "" : "." + String(fraction).padStart(9, "0").replace(/0+$/, "");
  return `${base}${digits}Z`;
}

/** Virtual instant, seconds after the fixture start. */
export function at(seconds) {
  return BigInt(epoch) * 1_000_000n + BigInt(Math.round(seconds * 1e9));
}

function stationSettings(id, overrides = {}) {
  return {
    id, enabled: true, latitudeDegrees: 50, longitudeDegrees: 14, siteElevationMetres: 250,
    antennaHeightMetres: 10, antennaGainDBi: 3, sensitivityDBm: -95, systemLossDB: 2,
    frameLossProbability: 0, ...overrides,
  };
}

function coverage(settings, referenceAltitudeFeet) {
  const horizon = 120 + settings.antennaHeightMetres / 10;
  const budget = settings.enabled ? 80 - settings.systemLossDB : 80 - settings.systemLossDB;
  return {
    referenceAltitudeFeet,
    horizonRadiusNauticalMiles: horizon,
    linkBudgetRadiusNauticalMiles: budget,
    effectiveRadiusNauticalMiles: Math.min(horizon, budget),
  };
}

function conflict(runId, message, field = "") {
  return { error: { code: "conflict", message, field, runId } };
}

// createRouter installs one route handler for prefix and dispatches by
// method and relative path; it records every call and supports barriers and
// one-shot failures.
async function createRouter(page, prefix, handlers) {
  const calls = [];
  const holds = [];
  const failures = [];
  await page.route(`**${prefix}**`, async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (!url.pathname.startsWith(prefix)) {
      return route.fallback();
    }
    const path = url.pathname.slice(prefix.length);
    const method = request.method();
    const text = request.postData();
    const body = text ? JSON.parse(text) : undefined;
    calls.push({ method, path, body });
    const hold = holds.findIndex((entry) => entry.method === method && entry.match(path));
    if (hold >= 0) {
      const [entry] = holds.splice(hold, 1);
      entry.arrived.release();
      await entry.gate.promise;
    }
    const failure = failures.findIndex((entry) => entry.method === method && entry.match(path));
    if (failure >= 0) {
      const [entry] = failures.splice(failure, 1);
      if (entry.abort) {
        return route.abort("failed").catch(() => {});
      }
      return json(route, entry.body, entry.status).catch(() => {});
    }
    const [status, payload] = handlers(method, path, body);
    return json(route, payload, status).catch(() => {});
  });

  const matcher = (path) => (typeof path === "string" ? (candidate) => candidate === path : (candidate) => path.test(candidate));
  return {
    calls,
    /** Number of calls with method and exact relative path. */
    count(method, path) {
      return calls.filter((call) => call.method === method && call.path === path).length;
    },
    /** Bodies of calls with method and path, in order. */
    bodies(method, path) {
      return calls.filter((call) => call.method === method && matcher(path)(call.path)).map((call) => call.body);
    },
    /**
     * Holds the next matching request until release(). Returns
     * { arrived, release }: arrived resolves when the request reached the
     * fake.
     */
    hold(method, path) {
      const entry = { method, match: matcher(path), gate: barrier(), arrived: barrier() };
      holds.push(entry);
      return { arrived: entry.arrived.promise, release: entry.gate.release };
    },
    /** Fails the next matching request with status and JSON body. */
    failNext(method, path, status, body) {
      failures.push({ method, match: matcher(path), status, body });
    },
    /** Aborts the next matching request at the network level. */
    abortNext(method, path) {
      failures.push({ method, match: matcher(path), abort: true });
    },
  };
}

/**
 * Installs a fake simulator API. options: { prefix, runId, initialCount,
 * speedHundredths, stations: [settings], referenceAltitudeFeet }.
 */
export async function fakeSimulator(page, options = {}) {
  const prefix = options.prefix ?? "/api/simulator/";
  const state = {
    runId: options.runId ?? "run-1",
    startNs: at(0),
    nowNs: at(0),
    initialCount: options.initialCount ?? 2,
    speed: options.speedHundredths ?? 100,
    aircraft: [],
    history: [],
    historyLimit: 4,
    sequence: 0n,
    stations: new Map(),
    reserved: new Set(),
    referenceAltitudeFeet: options.referenceAltitudeFeet ?? 10000,
    limits: { maxAircraft: 50, maxStations: 8, maxSpeedHundredths: 100000 },
  };

  function setCount(count) {
    state.aircraft = Array.from({ length: count }, (_, index) => ({
      icao: (0x4840d6 + index).toString(16).toUpperCase().padStart(6, "0"),
      callsign: `TB${String(index).padStart(4, "0")}`,
      createdAt: isoAt(state.startNs),
      latitudeDegrees: 50 + index / 10, longitudeDegrees: 14 + index / 10,
      barometricAltitudeFeet: 30000 + index * 1000, groundSpeedKnots: 420,
      trackDegrees: 90, verticalRateFeetPerMinute: 0,
    }));
  }

  function emit(kind, frame) {
    state.sequence += 1n;
    state.history.push({
      sequence: String(state.sequence), icao: "4840D6", timestamp: isoAt(state.nowNs), kind, frame,
    });
    while (state.history.length > state.historyLimit) {
      state.history.shift();
    }
  }

  function addStation(settings) {
    state.stations.set(settings.id, { settings: { ...settings }, revision: 1n, createdAt: isoAt(state.nowNs) });
    state.reserved.add(settings.id);
  }

  function stationWire(id) {
    const entry = state.stations.get(id);
    return { ...entry.settings, revision: String(entry.revision), createdAt: entry.createdAt };
  }

  setCount(state.initialCount);
  for (const settings of options.stations ?? [stationSettings("alpha")]) {
    addStation(settings);
  }

  const metadata = () => ({
    runId: state.runId, now: isoAt(state.nowNs), elapsedNanoseconds: String(state.nowNs - state.startNs),
    aircraftCount: state.aircraft.length, stationCount: state.stations.size,
    simulation: {
      id: state.runId, startTime: isoAt(state.startNs), seed: "42", initialAircraftCount: state.initialCount,
      speedHundredths: state.speed,
      spawn: {
        latitudeDegrees: { min: 49, max: 51 }, longitudeDegrees: { min: 13, max: 15 },
        altitudeFeet: { min: 1000, max: 40000 }, groundSpeedKnots: { min: 100, max: 500 },
        trackDegrees: { min: 0, max: 359 }, verticalRateFeetPerMinute: { min: -2000, max: 2000 },
      },
    },
    model: {
      transmitPowerDBm: 51, frequencyMHz: 1090, freeSpacePathLossConstantDB: 32.45,
      refractionFactor: 1.3333333333333333, horizonMetresPerSqrtMetre: 4120, earthRadiusMetres: 6371000,
    },
    limits: {
      ...state.limits, historyLimit: state.historyLimit, receptionHistoryLimit: 1000,
      maxHistoryPageSize: 1000, maxBatchFrames: 4096, maxBatchReceptions: 4096,
      maxAdvanceNanoseconds: "3600000000000",
    },
    driver: { heartbeatNanoseconds: "100000000", maxCatchUpNanoseconds: "1000000000" },
    service: {
      maxRequestBytes: 65536, maxResponseBytes: 1048576, requestTimeoutNanoseconds: "5000000000",
      coverageReferenceAltitudeFeet: state.referenceAltitudeFeet,
    },
  });

  const truth = () => ({
    runId: state.runId, now: isoAt(state.nowNs), elapsedNanoseconds: String(state.nowNs - state.startNs),
    initialAircraftCount: state.initialCount, aircraftCount: state.aircraft.length,
    speedHundredths: state.speed, aircraft: state.aircraft,
    history: {
      messages: state.history,
      oldestSequence: state.history.length ? state.history[0].sequence : "0",
      latestSequence: state.history.length ? state.history[state.history.length - 1].sequence : "0",
      limit: state.historyLimit,
    },
  });

  const stations = () => ({
    runId: state.runId, now: isoAt(state.nowNs),
    stations: [...state.stations.keys()].map((id) => ({
      station: stationWire(id), coverage: coverage(state.stations.get(id).settings, state.referenceAltitudeFeet),
    })),
  });

  const invalid = (message, field) => [400, { error: { code: "invalid", message, field, runId: state.runId } }];

  function handle(method, path, body) {
    if (method === "GET" && path === "metadata") {
      return [200, metadata()];
    }
    if (method === "GET" && path === "truth") {
      return [200, truth()];
    }
    if (method === "GET" && path === "stations") {
      return [200, stations()];
    }
    if (body && body.runId !== state.runId) {
      return [409, conflict(state.runId, `command run "${body.runId}" is not the served run`, "$.runId")];
    }
    if (method === "PUT" && path === "aircraft/count") {
      if (!Number.isInteger(body.count) || body.count < 0 || body.count > state.limits.maxAircraft) {
        return invalid(`count ${body.count} is outside [0,${state.limits.maxAircraft}]`, "$.count");
      }
      setCount(body.count);
      return [200, { runId: state.runId, operation: "setCount" }];
    }
    if (method === "PUT" && path === "time/speed") {
      if (!Number.isInteger(body.speedHundredths) || body.speedHundredths < 0 ||
          body.speedHundredths > state.limits.maxSpeedHundredths) {
        return invalid(`speed ${body.speedHundredths} is outside the accepted range`, "$.speedHundredths");
      }
      state.speed = body.speedHundredths;
      return [200, { runId: state.runId, operation: "setSpeed" }];
    }
    if (method === "POST" && path === "stations") {
      const settings = body.station;
      if (state.reserved.has(settings.id)) {
        return invalid(`station ID "${settings.id}" is already used in this run`, "$.station.id");
      }
      if (state.stations.size >= state.limits.maxStations) {
        return [422, { error: { code: "limit", message: "at most 8 stations may be active", field: "", runId: state.runId } }];
      }
      addStation(settings);
      return [201, { runId: state.runId, operation: "addStation", station: stationWire(settings.id) }];
    }
    const stationMatch = /^stations\/([^/]+)$/.exec(path);
    if (stationMatch) {
      const id = decodeURIComponent(stationMatch[1]);
      const entry = state.stations.get(id);
      if (!entry) {
        return [404, { error: { code: "not_found", message: `station "${id}" not found`, field: "", runId: state.runId } }];
      }
      if (body.expectedRevision !== String(entry.revision)) {
        return [409, conflict(state.runId,
          `station "${id}" is at revision ${entry.revision}, not ${body.expectedRevision}`, "$.expectedRevision")];
      }
      if (method === "DELETE") {
        state.stations.delete(id);
        return [200, { runId: state.runId, operation: "removeStation" }];
      }
      if (method === "PUT") {
        if (body.station.id !== id) {
          return invalid("the station ID cannot be changed", "$.station.id");
        }
        entry.settings = { ...body.station };
        entry.revision += 1n;
        return [200, { runId: state.runId, operation: "updateStation", station: stationWire(id) }];
      }
    }
    return [404, { error: { code: "not_found", message: `no route "${path}"`, field: "", runId: state.runId } }];
  }

  const router = await createRouter(page, prefix, handle);
  return Object.assign(router, {
    state,
    stationSettings,
    /** Advances virtual time and emits one generated frame. */
    advance(seconds, frame = frames.identification, kind = "identification") {
      if (state.speed > 0) {
        state.nowNs += BigInt(Math.round(seconds * 1e9));
      }
      emit(kind, frame);
    },
    /** Replaces the run: new identity, fresh clock and stations. */
    restart(runId) {
      state.runId = runId;
      state.nowNs = state.startNs;
      state.history = [];
      state.sequence = 0n;
      state.stations = new Map();
      state.reserved = new Set();
      addStation(stationSettings("alpha"));
    },
    /** Replaces one station's settings as another client would. */
    editStation(id, changes) {
      const entry = state.stations.get(id);
      entry.settings = { ...entry.settings, ...changes };
      entry.revision += 1n;
    },
    removeStation(id) {
      state.stations.delete(id);
    },
  });
}

/** Builds one reception record of a received frame. */
export function reception({ sequence, transmissionSequence, stationId = "alpha", revision = "1", icao = "4840D6", kind, timestamp, frame, receiver = {} }) {
  return {
    sequence: String(sequence), transmissionSequence: String(transmissionSequence), stationId,
    stationRevision: revision, icao, kind, timestamp, frame,
    slantRangeNauticalMiles: 42.5, receivedPowerDBm: -71.25,
    receiver: {
      ...stationSettings(stationId), revision, createdAt: isoAt(at(0)), ...receiver,
    },
  };
}

/** Builds one Evidence object with its receiver copies. */
export function evidence({ transmissionSequence, icao = "4840D6", kind, timestamp, frame, receptions }) {
  return { transmissionSequence: String(transmissionSequence), icao, kind, timestamp, frame, receptions };
}

/**
 * Builds a received aircraft. fields: { icao, lastReceivedAt, identity,
 * position, altitude, velocity } where each field is null or an object with
 * the observation values; evidence is generated.
 */
export function aircraft({ icao = "4840D6", lastReceivedAt, identity = null, position = null, altitude = null, velocity = null, stationIds = ["alpha"] }) {
  let transmission = 0;
  const copies = (kind, timestamp, frame) => {
    transmission += 1;
    return evidence({
      transmissionSequence: transmission, icao, kind, timestamp, frame,
      receptions: stationIds.map((stationId, index) => reception({
        sequence: transmission * 10 + index, transmissionSequence: transmission, stationId, icao, kind, timestamp, frame,
      })),
    });
  };
  const result = { icao, lastReceivedAt, identity: null, position: null, barometricAltitude: null, velocity: null };
  if (identity) {
    result.identity = { callsign: identity.callsign, observedAt: identity.observedAt,
      evidence: copies("identification", identity.observedAt, frames.identification) };
  }
  if (position) {
    result.position = {
      latitudeDegrees: position.latitudeDegrees, longitudeDegrees: position.longitudeDegrees, observedAt: position.observedAt,
      evidence: [copies("position", position.observedAt, frames.positionEven), copies("position", position.observedAt, frames.positionOdd)],
    };
  }
  if (altitude) {
    result.barometricAltitude = { feet: altitude.feet, observedAt: altitude.observedAt,
      evidence: copies("position", altitude.observedAt, frames.positionEven) };
  }
  if (velocity) {
    result.velocity = {
      subtype: 1, intentChange: false, ifrCapability: false, nacv: 1,
      eastKnots: { value: 100, overRange: false }, northKnots: { value: 0, overRange: false },
      groundSpeedKnots: null, trackDegrees: null, headingDegrees: null, airspeedKnots: null,
      trueAirspeed: false, barometricVerticalRate: true,
      verticalRateFeetPerMinute: { value: 0, overRange: false }, gnssMinusBaroFeet: null,
      observedAt: velocity.observedAt,
      evidence: copies("velocity", velocity.observedAt, frames.velocity),
      ...velocity.values,
    };
  }
  return result;
}

const expiry = {
  identityNanoseconds: "60000000000", positionNanoseconds: "30000000000",
  altitudeNanoseconds: "30000000000", velocityNanoseconds: "30000000000",
};

/**
 * Installs a fake display backend. options: { prefix, runId, stations,
 * aircraftFor(stationIds, state) }. state.nowNs is the virtual instant of
 * the next observation snapshot.
 */
export async function fakeDisplay(page, options = {}) {
  const prefix = options.prefix ?? "/api/display/";
  const state = {
    runId: options.runId ?? "run-1",
    nowNs: at(10),
    referenceAltitudeFeet: 10000,
    stations: options.stations ?? [stationSettings("alpha"), stationSettings("bravo", { latitudeDegrees: 51 })],
    revisions: {},
    aircraftFor: options.aircraftFor ?? (() => []),
    history: options.history ?? null,
  };

  const stations = () => ({
    runId: state.runId, now: isoAt(state.nowNs),
    stations: state.stations.map((settings) => ({
      station: { ...settings, revision: state.revisions[settings.id] ?? "1", createdAt: isoAt(at(0)) },
      coverage: coverage(settings, state.referenceAltitudeFeet),
    })),
  });

  function observations(stationIds) {
    const selection = [...new Set(stationIds)].sort();
    return {
      runId: state.runId, now: isoAt(state.nowNs), stationIds: selection,
      retention: selection.map((stationId) => ({
        stationId, oldestSequence: "1", latestSequence: "20", truncated: false, limit: 1000,
      })),
      expiry,
      aircraft: state.aircraftFor(selection, state),
    };
  }

  function handle(method, path, body) {
    if (method === "GET" && path === "stations") {
      return [200, stations()];
    }
    if (method === "GET" && path === "snapshot") {
      return [200, { status: "unavailable", lastUpdatedAt: null, observations: null, error: null }];
    }
    if (method === "POST" && path === "observations") {
      const unknown = body.stationIds.find((id) => !state.stations.some((station) => station.id === id));
      if (unknown) {
        return [404, {
          snapshot: { status: "unavailable", lastUpdatedAt: null, observations: null, error: null },
          error: { code: "not_found", message: `station "${unknown}" not found`, field: "read reception snapshot", runId: state.runId },
        }];
      }
      return [200, {
        snapshot: { status: "fresh", lastUpdatedAt: "2030-01-01T00:00:00Z", observations: observations(body.stationIds), error: null },
        error: null,
      }];
    }
    if (method === "POST" && path === "receptions/history") {
      if (!state.history) {
        return [404, { error: { code: "not_found", message: "no history", field: "", runId: state.runId } }];
      }
      return state.history(body, state);
    }
    return [404, { error: { code: "not_found", message: `no route "${path}"`, field: "", runId: state.runId } }];
  }

  const router = await createRouter(page, prefix, handle);
  return Object.assign(router, { state, observations, stationSettings });
}

/**
 * A mutable per-station reception history with the engine's paging rules:
 * records oldest..latest are retained; a request after sequence `after`
 * returns up to `limit` newer records; gap is set when a cursor points
 * before the oldest retained record; a cursor from another run is a 409.
 * Set store.oldest/latest to simulate eviction and new receptions, and
 * store.revisionOf(sequence) to vary historical receiver revisions.
 */
export function historyStore({ stationId = "alpha", oldest = 1n, latest = 10n, retentionLimit = 1000 } = {}) {
  const store = {
    stationId, oldest, latest, retentionLimit,
    revisionOf: () => "1",
    receiverOf: () => ({}),
    handler(body, state) {
      if (!state.stations.some((station) => station.id === body.stationId) || body.stationId !== store.stationId) {
        return [404, { error: { code: "not_found", message: `station "${body.stationId}" not found`, field: "read reception history", runId: state.runId } }];
      }
      if (body.cursor && body.cursor.runId !== state.runId) {
        return [409, { error: { code: "conflict", message: `cursor run "${body.cursor.runId}" does not match current run "${state.runId}"`,
          field: "read reception history", runId: state.runId } }];
      }
      const after = body.cursor ? BigInt(body.cursor.afterSequence) : 0n;
      const records = [];
      for (let sequence = after + 1n > store.oldest ? after + 1n : store.oldest;
        sequence <= store.latest && records.length < body.limit; sequence++) {
        const revision = store.revisionOf(sequence);
        records.push(reception({
          sequence, transmissionSequence: sequence, stationId: body.stationId, revision, icao: "4840D6",
          kind: "identification", timestamp: isoAt(at(Number(sequence % 1000n))), frame: frames.identification,
          receiver: store.receiverOf(sequence),
        }));
      }
      const next = records.length > 0 ? records[records.length - 1].sequence : String(after);
      return [200, {
        runId: state.runId, stationId: body.stationId, now: isoAt(state.nowNs), records,
        oldestSequence: String(store.oldest), latestSequence: String(store.latest),
        nextCursor: { runId: state.runId, stationId: body.stationId, afterSequence: next },
        gap: body.cursor !== null && store.oldest > 0n && after < store.oldest - 1n,
        hasMore: BigInt(next) < store.latest, retentionLimit: store.retentionLimit,
      }];
    },
  };
  return store;
}
