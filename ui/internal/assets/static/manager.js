// Manager component: simulator truth, traffic and clock controls, generated
// frames and the station editor.
//
// Every refresh reads GET metadata and GET truth beneath the simulator API
// base and accepts them only when both name the same run. Commands are
// absolute assignments carrying the observed run ID: PUT aircraft/count and
// PUT time/speed. A command acknowledgement is not a state snapshot; after
// one the component refreshes and shows what the simulator reports. Drafts
// stay separate from effective values and survive failures and polling.
//
// Virtual time (the simulator clock) and real update time (when this
// browser last refreshed successfully) are labeled separately. Pausing or a
// failed refresh never advances the displayed virtual time.

import { createPoller, ErrorKind, requestJSON } from "./client.js";
import { busyWhile, createScaffold, ensureStylesheet, showError } from "./component.js";
import { validateManagerConfig } from "./config.js";
import { h, replaceChildren, setText, uniqueId } from "./dom.js";
import { mountStationEditor } from "./stations.js";
import { formatHundredths, formatNanoseconds, parseDecimal, parseTimestamp } from "./time.js";

const integerPattern = /^(0|[1-9][0-9]*)$/;

/** Parses a draft as a nonnegative integer within [0, max], or returns an error message. */
export function parseIntegerDraft(text, max, name) {
  const trimmed = text.trim();
  if (trimmed === "") {
    return { error: `${name} is required` };
  }
  if (!integerPattern.test(trimmed)) {
    return { error: `${name} must be a whole number without sign, decimals or leading zeros` };
  }
  const value = Number(trimmed);
  if (!Number.isSafeInteger(value) || value > max) {
    return { error: `${name} must be within 0..${max}` };
  }
  return { value };
}

// validTruth checks the parts of a truth snapshot this component renders,
// so a malformed payload is reported instead of breaking rendering.
function validTruth(truth) {
  try {
    parseTimestamp(truth.now);
    parseDecimal(truth.elapsedNanoseconds);
    parseDecimal(truth.history.oldestSequence);
    parseDecimal(truth.history.latestSequence);
    return typeof truth.runId === "string" && truth.runId !== "" && Array.isArray(truth.aircraft) &&
      Array.isArray(truth.history.messages) && Number.isSafeInteger(truth.speedHundredths) &&
      Number.isSafeInteger(truth.aircraftCount) && Number.isSafeInteger(truth.initialAircraftCount);
  } catch {
    return false;
  }
}

function validMetadata(metadata) {
  return typeof metadata?.runId === "string" && Number.isSafeInteger(metadata?.limits?.maxAircraft) &&
    Number.isSafeInteger(metadata?.limits?.maxSpeedHundredths);
}

function fact(term, value) {
  return [h("dt", {}, term), h("dd", {}, value)];
}

function formatNumber(value, digits) {
  return typeof value === "number" && Number.isFinite(value) ? value.toFixed(digits) : "unavailable";
}

/** Mounts a manager into root. See components.js for the contract. */
export function mountManager(root, rawConfig) {
  const config = validateManagerConfig(rawConfig, window.location.origin);
  ensureStylesheet(config.assetBaseUrl + "ui.css");
  const { lifecycle, wrapper, status, error, content } = createScaffold(root, { kind: "manager", label: "Simulator manager" });
  const api = config.apiBaseUrl;
  const request = (path, options = {}) => {
    const controller = lifecycle.controller();
    const forward = () => controller.abort();
    options.signal?.addEventListener("abort", forward, { once: true });
    return requestJSON(api + path, {
      ...options, signal: controller.signal,
      timeoutMs: config.requestTimeoutMilliseconds, maxBytes: config.maxResponseBytes,
    }).finally(() => {
      options.signal?.removeEventListener("abort", forward);
      controller.release();
    });
  };

  const state = {
    runId: null,
    metadata: null,
    truth: null,
    stale: false,
    lastUpdatedAt: null,
    lastPositiveSpeed: null,
    generation: 0,
    reviewRun: null,
  };

  // Run and clock facts.
  const runFacts = h("dl", { class: "tb-facts" });
  const staleNote = h("p", { class: "tb-stale", hidden: true },
    "Showing the last successful simulator state. Controls are disabled until a refresh succeeds.");
  const reviewText = h("span");
  const reviewButton = h("button", { type: "button" }, "Drafts reviewed");
  const reviewNote = h("div", { class: "tb-notice", hidden: true }, reviewText, " ", reviewButton);
  const runPanel = h("section", { class: "tb-panel", "aria-label": "Run and clock" },
    h("h2", {}, "Run and clock"), staleNote, reviewNote, runFacts);

  // Controls.
  const controls = {};
  function control(key, labelText, unit, buttonText) {
    const id = uniqueId(`tb-${key}`);
    const input = h("input", { id, type: "text", inputmode: "numeric", autocomplete: "off", "aria-describedby": `${id}-hint ${id}-error` });
    const hint = h("span", { id: `${id}-hint`, class: "tb-unit" }, unit);
    const fieldError = h("span", { id: `${id}-error`, class: "tb-field-error", role: "alert" });
    const dirtyNote = h("span", { class: "tb-dirty", hidden: true }, "Unsaved draft");
    const button = h("button", { type: "submit" }, buttonText);
    const form = h("form", {},
      h("label", { for: id }, labelText, hint, input), dirtyNote, fieldError, h("div", { class: "tb-actions" }, button));
    controls[key] = { input, fieldError, dirtyNote, button, form, pending: false, dirty: false };
    lifecycle.listen(input, "input", () => {
      controls[key].dirty = input.value !== "";
      render();
    });
    return form;
  }
  const countForm = control("count", "Aircraft count", "Absolute number of simulated aircraft; 0 removes all", "Set count");
  const speedForm = control("speed", "Speed", "Hundredths of real time: 100 is real time, 0 pauses", "Set speed");
  const pauseButton = h("button", { type: "button" }, "Pause");
  const resumeButton = h("button", { type: "button" }, "Resume");
  const clockActions = h("div", { class: "tb-actions", role: "group", "aria-label": "Clock" }, pauseButton, resumeButton);
  const commandStatus = h("p", { class: "tb-status", role: "status", "aria-live": "polite" });
  const commandError = h("div", { class: "tb-error", role: "alert", hidden: true });
  const controlPanel = h("section", { class: "tb-panel", "aria-label": "Controls" },
    h("h2", {}, "Controls"), h("div", { class: "tb-grid" }, countForm, speedForm), clockActions, commandStatus, commandError);

  // Truth and generated frames.
  const truthBody = h("tbody");
  const truthEmpty = h("p", { class: "tb-muted", hidden: true }, "No simulated aircraft.");
  const truthPanel = h("section", { class: "tb-panel", "aria-label": "Simulator truth" },
    h("h2", {}, "Simulator truth"),
    h("p", { class: "tb-muted" }, "Exact simulated state. It is not received data; speeds are aircraft speeds, not scaled by the clock."),
    truthEmpty,
    h("div", { class: "tb-scroll" }, h("table", {},
      h("thead", {}, h("tr", {}, ...["ICAO", "Callsign", "Latitude (deg)", "Longitude (deg)", "Pressure altitude (ft)",
        "Ground speed (kt)", "Track (deg)", "Vertical rate (ft/min)"].map((title) => h("th", { scope: "col" }, title)))),
      truthBody)));
  const framesBody = h("tbody");
  const framesBounds = h("p", { class: "tb-muted" });
  const framesPanel = h("section", { class: "tb-panel", "aria-label": "Generated frames" },
    h("h2", {}, "Generated frames"),
    h("p", { class: "tb-muted" }, "Frames the simulator transmitted, before any reception model. Not received evidence."),
    framesBounds,
    h("div", { class: "tb-scroll" }, h("table", {},
      h("thead", {}, h("tr", {}, ...["Sequence", "Virtual time", "ICAO", "Kind", "Frame"].map((title) => h("th", { scope: "col" }, title)))),
      framesBody)));

  const stationsPanel = h("section", { class: "tb-panel", "aria-label": "Stations" });
  content.append(runPanel, controlPanel, stationsPanel, truthPanel, framesPanel);

  const stations = mountStationEditor(stationsPanel, {
    lifecycle, request,
    currentRun: () => (state.stale || state.reviewRun ? null : state.runId),
    maxStations: () => state.metadata?.limits?.maxStations ?? 8,
    refreshNow: () => poller.refreshNow(),
  });

  function canSubmit() {
    return state.runId !== null && !state.stale && state.reviewRun === null;
  }

  function render() {
    const truth = state.truth;
    staleNote.hidden = !state.stale || truth === null;
    if (state.reviewRun) {
      setText(reviewText,
        `The simulator restarted as run ${state.runId}. Drafts written for run ${state.reviewRun} are kept. Review them before submitting.`);
      reviewNote.hidden = false;
    } else {
      reviewNote.hidden = true;
    }
    if (truth) {
      const paused = truth.speedHundredths === 0;
      const facts = [
        ...fact("Run ID", truth.runId),
        ...fact("Virtual time", truth.now),
        ...fact("Elapsed virtual time", formatNanoseconds(parseDecimal(truth.elapsedNanoseconds))),
        ...fact("Clock", paused ? "Paused" : `Running at ${formatHundredths(truth.speedHundredths)} real time`),
        ...fact("Speed (hundredths)", String(truth.speedHundredths)),
        ...fact("Aircraft count", String(truth.aircraftCount)),
        ...fact("Initial aircraft count", String(truth.initialAircraftCount)),
      ];
      if (state.metadata) {
        facts.push(...fact("Limits", `count 0..${state.metadata.limits.maxAircraft}, speed 0..${state.metadata.limits.maxSpeedHundredths} hundredths`));
      }
      facts.push(...fact("Last successful update (real time)", state.lastUpdatedAt === null ? "never"
        : `${new Date(state.lastUpdatedAt).toISOString()} (${Math.max(0, Math.round((Date.now() - state.lastUpdatedAt) / 1000))} s ago)`));
      replaceChildren(runFacts, ...facts);

      truthEmpty.hidden = truth.aircraft.length > 0;
      replaceChildren(truthBody, ...truth.aircraft.map((aircraft) => h("tr", {},
        h("td", {}, aircraft.icao), h("td", {}, aircraft.callsign),
        h("td", {}, formatNumber(aircraft.latitudeDegrees, 5)), h("td", {}, formatNumber(aircraft.longitudeDegrees, 5)),
        h("td", {}, formatNumber(aircraft.barometricAltitudeFeet, 0)), h("td", {}, formatNumber(aircraft.groundSpeedKnots, 1)),
        h("td", {}, formatNumber(aircraft.trackDegrees, 1)), h("td", {}, formatNumber(aircraft.verticalRateFeetPerMinute, 0)))));

      const history = truth.history;
      const oldest = parseDecimal(history.oldestSequence);
      const bounds = history.messages.length === 0
        ? `No generated frames retained (history limit ${history.limit}).`
        : `Retained sequences ${history.oldestSequence}..${history.latestSequence} (history limit ${history.limit}).`;
      setText(framesBounds, oldest > 1n ? `${bounds} Frames before sequence ${history.oldestSequence} are no longer retained.` : bounds);
      replaceChildren(framesBody, ...history.messages.map((message) => h("tr", {},
        h("td", {}, message.sequence), h("td", {}, message.timestamp), h("td", {}, message.icao),
        h("td", {}, message.kind), h("td", { class: "tb-frame" }, message.frame))));
    } else {
      replaceChildren(runFacts, ...fact("Run ID", "unavailable"));
      truthEmpty.hidden = true;
      replaceChildren(truthBody);
      setText(framesBounds, "");
      replaceChildren(framesBody);
    }

    for (const [key, entry] of Object.entries(controls)) {
      const current = key === "count" ? truth?.aircraftCount : truth?.speedHundredths;
      entry.dirtyNote.hidden = !entry.dirty;
      setText(entry.dirtyNote, entry.dirty && current !== undefined ? `Unsaved draft; current value is ${current}` : "Unsaved draft");
      entry.button.disabled = entry.pending || !canSubmit();
      entry.input.placeholder = current === undefined ? "" : `current: ${current}`;
    }
    const clockPending = controls.speed.pending;
    pauseButton.disabled = clockPending || !canSubmit() || truth?.speedHundredths === 0;
    resumeButton.disabled = clockPending || !canSubmit() || (truth !== null && truth.speedHundredths > 0);
    setText(resumeButton, `Resume at ${formatHundredths(state.lastPositiveSpeed ?? config.resumeSpeedHundredths)}`);
    stations.render();
  }

  lifecycle.listen(reviewButton, "click", () => {
    state.reviewRun = null;
    render();
  });

  function acceptRun(runId) {
    if (state.runId !== null && state.runId !== runId) {
      // A replacement run: nothing from the previous run may be shown or
      // submitted without review.
      state.truth = null;
      state.lastPositiveSpeed = null;
      const dirty = Object.values(controls).some((entry) => entry.dirty) || stations.hasDirtyDrafts();
      state.reviewRun = dirty ? state.runId : null;
    }
    stations.useRun(runId);
    state.runId = runId;
  }

  async function refresh(signal) {
    const generation = state.generation;
    const [metadata, truth] = await Promise.all([request("metadata", { signal }), request("truth", { signal })]);
    if (lifecycle.destroyed || signal.aborted || generation !== state.generation) {
      return;
    }
    const failure = !metadata.ok ? metadata.error : !truth.ok ? truth.error : null;
    if (failure) {
      state.stale = true;
      setText(status, "Simulator state is stale: the last refresh failed.");
      showError(error, failure, "Refreshing the simulator failed");
      render();
      return;
    }
    if (!validMetadata(metadata.body) || !validTruth(truth.body)) {
      state.stale = true;
      showError(error, { message: "the simulator returned an invalid metadata or truth payload" }, "Refreshing the simulator failed");
      render();
      return;
    }
    if (metadata.body.runId !== truth.body.runId) {
      // Metadata and truth are separate reads; a restart between them is
      // not a coherent state, so wait for a refresh that agrees.
      state.stale = true;
      showError(error, { message: "the simulator run changed during the refresh; refreshing again" }, "Refreshing the simulator failed");
      render();
      poller.refreshNow();
      return;
    }
    acceptRun(truth.body.runId);
    state.metadata = metadata.body;
    state.truth = truth.body;
    state.stale = false;
    state.lastUpdatedAt = Date.now();
    if (truth.body.speedHundredths > 0) {
      state.lastPositiveSpeed = truth.body.speedHundredths;
    }
    showError(error, null);
    setText(status, "Simulator state is current.");
    await stations.refresh(signal);
    if (!lifecycle.destroyed) {
      render();
    }
  }

  // submit sends one absolute command with the observed run ID. It never
  // retries; a timed-out command has an unknown outcome and requires a new
  // user submission after the refresh.
  async function submit(key, path, body, describe) {
    const entry = controls[key];
    entry.pending = true;
    state.generation += 1;
    showError(commandError, null);
    setText(commandStatus, `Sending ${describe}...`);
    render();
    const result = await request(path, { method: "PUT", body: { runId: state.runId, ...body } });
    if (lifecycle.destroyed) {
      return false;
    }
    entry.pending = false;
    if (result.ok) {
      setText(commandStatus, `${describe} accepted. Refreshing simulator state.`);
    } else if (result.error.kind === ErrorKind.timeout || result.error.kind === ErrorKind.network) {
      setText(commandStatus, "");
      showError(commandError, { message: `the outcome of ${describe} is unknown (${result.error.message}). Check the refreshed state and submit again if needed.` });
    } else {
      setText(commandStatus, "");
      showError(commandError, result.error, `${describe} was rejected`);
      if (result.error.field) {
        setText(entry.fieldError, result.error.message);
      }
    }
    poller.refreshNow();
    render();
    return result.ok;
  }

  function bindSubmit(key, name, max, path, bodyOf) {
    const entry = controls[key];
    lifecycle.listen(entry.form, "submit", async (event) => {
      event.preventDefault();
      if (entry.pending || !canSubmit()) {
        return;
      }
      const parsed = parseIntegerDraft(entry.input.value, max(), name);
      if (parsed.error) {
        setText(entry.fieldError, parsed.error);
        return;
      }
      setText(entry.fieldError, "");
      const submitted = entry.input.value;
      const accepted = await submit(key, path, bodyOf(parsed.value), `${name} ${parsed.value}`);
      // Clear the draft only if the user did not type a newer one while
      // the command was pending.
      if (accepted && !lifecycle.destroyed && entry.input.value === submitted) {
        entry.input.value = "";
        entry.dirty = false;
        render();
      }
    });
  }
  bindSubmit("count", "Aircraft count", () => state.metadata.limits.maxAircraft, "aircraft/count", (count) => ({ count }));
  bindSubmit("speed", "Speed", () => state.metadata.limits.maxSpeedHundredths, "time/speed", (speedHundredths) => ({ speedHundredths }));

  lifecycle.listen(pauseButton, "click", () => {
    if (!controls.speed.pending && canSubmit()) {
      submit("speed", "time/speed", { speedHundredths: 0 }, "Pause");
    }
  });
  lifecycle.listen(resumeButton, "click", () => {
    if (!controls.speed.pending && canSubmit()) {
      const speed = state.lastPositiveSpeed ?? config.resumeSpeedHundredths;
      submit("speed", "time/speed", { speedHundredths: speed }, `Resume at ${formatHundredths(speed)}`);
    }
  });

  const poller = createPoller({ intervalMs: config.pollIntervalMilliseconds, run: busyWhile(wrapper, lifecycle, refresh) });
  lifecycle.onDestroy(() => poller.stop());
  render();
  poller.start();
  return {
    destroy: () => lifecycle.destroy(),
    get pendingRequests() {
      return lifecycle.pendingRequests;
    },
  };
}
