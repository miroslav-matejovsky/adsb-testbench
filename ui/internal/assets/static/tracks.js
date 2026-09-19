// DOM-free received-track model for the aircraft display.
//
// A view is built from one validated ObservationSnapshot at its own virtual
// instant `now`. Field ages are `now - observedAt` in exact BigInt
// nanoseconds; the backend already removed expired fields, so a null field
// is unavailable and nothing is carried forward from an earlier snapshot.
// Track status applies the configured thresholds to `now - lastReceivedAt`:
//
//   fresh  age <= freshFor
//   stale  freshFor < age <= lostAfter
//   lost   age > lostAfter
//
// Status is view policy and never extends a field lifetime. An aircraft that
// disappears between two successful snapshots of the same selection and run
// becomes a tombstone for exactly one view: identity and last-seen labels
// only, never an old measurement. Wall-clock time is never used here.

import { parseDecimal, parseTimestamp } from "./time.js";

const icaoPattern = /^[0-9A-F]{6}$/;

/** Returns a sorted copy of a selection without duplicates. */
export function normalizeSelection(stationIds) {
  return [...new Set(stationIds)].sort();
}

/** Stable key of a normalized selection. */
export function selectionKey(stationIds) {
  return JSON.stringify(normalizeSelection(stationIds));
}

/** Classifies a track by the virtual age of its last received frame. */
export function trackStatus(ageNs, freshForNs, lostAfterNs) {
  if (ageNs <= freshForNs) {
    return "fresh";
  }
  return ageNs <= lostAfterNs ? "stale" : "lost";
}

function finite(value) {
  return typeof value === "number" && Number.isFinite(value);
}

function measurement(value) {
  if (value === null) {
    return null;
  }
  if (!value || !finite(value.value) || typeof value.overRange !== "boolean") {
    throw new TypeError("invalid measurement");
  }
  return { value: value.value, overRange: value.overRange };
}

function optionalNumber(value) {
  if (value === null) {
    return null;
  }
  if (!finite(value)) {
    throw new TypeError("invalid number");
  }
  return value;
}

function validEvidence(evidence) {
  if (!evidence || !Array.isArray(evidence.receptions) || typeof evidence.frame !== "string") {
    throw new TypeError("invalid evidence");
  }
  parseDecimal(evidence.transmissionSequence);
  parseTimestamp(evidence.timestamp);
  return evidence;
}

// field returns the observed instant and age of one received field, or null.
function field(value, nowNs) {
  if (value === null) {
    return null;
  }
  const observedNs = parseTimestamp(value.observedAt);
  if (observedNs > nowNs) {
    throw new TypeError("field observed after the snapshot instant");
  }
  return { observedAt: value.observedAt, ageNs: nowNs - observedNs };
}

// receivers lists the distinct stations that contributed evidence.
function receivers(aircraft) {
  const ids = new Set();
  const evidences = [
    aircraft.identity?.evidence, aircraft.barometricAltitude?.evidence, aircraft.velocity?.evidence,
    ...(aircraft.position?.evidence ?? []),
  ].filter(Boolean);
  for (const evidence of evidences) {
    for (const copy of evidence.receptions) {
      ids.add(copy.stationId);
    }
  }
  return [...ids].sort();
}

/** Builds one row from a validated received aircraft. */
function row(aircraft, nowNs, thresholds) {
  if (!icaoPattern.test(aircraft.icao)) {
    throw new TypeError(`invalid ICAO ${aircraft.icao}`);
  }
  const lastNs = parseTimestamp(aircraft.lastReceivedAt);
  if (lastNs > nowNs) {
    throw new TypeError("aircraft received after the snapshot instant");
  }
  const ageNs = nowNs - lastNs;
  const identity = aircraft.identity;
  const position = aircraft.position;
  const altitude = aircraft.barometricAltitude;
  const velocity = aircraft.velocity;
  if (identity) {
    validEvidence(identity.evidence);
  }
  if (position) {
    if (!finite(position.latitudeDegrees) || !finite(position.longitudeDegrees) ||
        !Array.isArray(position.evidence) || position.evidence.length === 0) {
      throw new TypeError("invalid position");
    }
    position.evidence.forEach(validEvidence);
  }
  if (altitude) {
    optionalNumber(altitude.feet);
    validEvidence(altitude.evidence);
  }
  let motion = null;
  if (velocity) {
    validEvidence(velocity.evidence);
    motion = {
      subtype: velocity.subtype,
      groundSpeedKnots: optionalNumber(velocity.groundSpeedKnots),
      trackDegrees: optionalNumber(velocity.trackDegrees),
      headingDegrees: optionalNumber(velocity.headingDegrees),
      airspeed: measurement(velocity.airspeedKnots),
      trueAirspeed: velocity.trueAirspeed === true,
      eastKnots: measurement(velocity.eastKnots),
      northKnots: measurement(velocity.northKnots),
      verticalRate: measurement(velocity.verticalRateFeetPerMinute),
      barometricVerticalRate: velocity.barometricVerticalRate === true,
    };
  }
  return {
    icao: aircraft.icao,
    tombstone: false,
    status: trackStatus(ageNs, thresholds.freshForNanoseconds, thresholds.lostAfterNanoseconds),
    lastReceivedAt: aircraft.lastReceivedAt,
    ageNs,
    identity: identity ? { callsign: identity.callsign, ...field(identity, nowNs) } : null,
    position: position ? {
      latitudeDegrees: position.latitudeDegrees, longitudeDegrees: position.longitudeDegrees, ...field(position, nowNs),
    } : null,
    altitude: altitude ? { feet: altitude.feet, ...field(altitude, nowNs) } : null,
    velocity: motion ? { ...motion, ...field(velocity, nowNs) } : null,
    receivers: receivers(aircraft),
    source: aircraft,
  };
}

/**
 * Validates an ObservationSnapshot against the exact normalized selection
 * that was requested. Throws TypeError describing the first problem.
 */
export function validateObservations(observations, selection) {
  if (!observations || typeof observations !== "object") {
    throw new TypeError("observations are missing");
  }
  if (typeof observations.runId !== "string" || observations.runId === "") {
    throw new TypeError("runId is missing");
  }
  parseTimestamp(observations.now);
  if (!Array.isArray(observations.stationIds) || !Array.isArray(observations.aircraft) ||
      !Array.isArray(observations.retention)) {
    throw new TypeError("stationIds, retention and aircraft must be arrays");
  }
  if (JSON.stringify(normalizeSelection(observations.stationIds)) !== JSON.stringify(normalizeSelection(selection))) {
    throw new TypeError("the response belongs to a different station selection");
  }
}

/**
 * Builds the view of one validated snapshot. previous is the view of the
 * immediately preceding successful snapshot of the same selection and run,
 * or null; it only contributes tombstones for aircraft that disappeared, and
 * previous tombstones are dropped. Rows are sorted by ICAO.
 */
export function buildView(observations, previous, thresholds) {
  const nowNs = parseTimestamp(observations.now);
  const rows = observations.aircraft.map((aircraft) => row(aircraft, nowNs, thresholds));
  const present = new Set(rows.map((entry) => entry.icao));
  if (previous) {
    for (const old of previous.rows) {
      if (!old.tombstone && !present.has(old.icao)) {
        rows.push({
          icao: old.icao, tombstone: true, status: "lost", lastReceivedAt: old.lastReceivedAt,
          ageNs: nowNs - parseTimestamp(old.lastReceivedAt), callsign: old.identity?.callsign ?? null,
          identity: null, position: null, altitude: null, velocity: null, receivers: [], source: null,
        });
      }
    }
  }
  rows.sort((a, b) => (a.icao < b.icao ? -1 : a.icao > b.icao ? 1 : 0));
  return {
    runId: observations.runId, now: observations.now, nowNs,
    selection: normalizeSelection(observations.stationIds), retention: observations.retention,
    expiry: observations.expiry, rows,
  };
}
