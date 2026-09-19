// Station editor used by the manager component.
//
// Stations are created, replaced, enabled/disabled and removed with complete
// settings through the existing simulator routes:
//
//   POST   stations        {runId, station}                     -> 201
//   PUT    stations/{id}   {runId, expectedRevision, station}   -> 200
//   DELETE stations/{id}   {runId, expectedRevision}            -> 200
//
// Each form keeps its draft separately from the latest server station,
// together with the run and revision it was written against. Polling only
// refreshes clean forms. A dirty form whose station changed, a revision
// conflict, a removed station, or a replaced run keeps the draft and shows
// the server values beside it; submitting again requires an explicit
// "Reapply draft to current revision" and a new submit. Nothing is retried
// automatically and the latest revision is never substituted silently.

import { showError } from "./component.js";
import { h, replaceChildren, setText, uniqueId } from "./dom.js";

const stationIdPattern = /^[A-Za-z0-9_-]{1,64}$/;
const numberPattern = /^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][-+]?[0-9]+)?$/;

/** Every numeric station setting with its unit and accepted domain. */
export const stationFields = Object.freeze([
  { key: "latitudeDegrees", label: "Latitude", unit: "degrees", min: -90, max: 90 },
  { key: "longitudeDegrees", label: "Longitude", unit: "degrees", min: -180, max: 180 },
  { key: "siteElevationMetres", label: "Site elevation", unit: "metres", min: -500, max: 9000 },
  { key: "antennaHeightMetres", label: "Antenna height", unit: "metres above site", min: 0, max: 500 },
  { key: "antennaGainDBi", label: "Antenna gain", unit: "dBi", min: -10, max: 40 },
  { key: "sensitivityDBm", label: "Sensitivity", unit: "dBm", min: -140, max: 0 },
  { key: "systemLossDB", label: "System loss", unit: "dB", min: 0, max: 30 },
  { key: "frameLossProbability", label: "Frame loss probability", unit: "probability 0..1", min: 0, max: 1 },
].map(Object.freeze));

/** Returns the relative route of one station, encoded as one path segment. */
export function stationPath(id) {
  return `stations/${encodeURIComponent(id)}`;
}

/**
 * Validates draft text values and returns { settings } with explicit
 * numbers and booleans, or { errors } keyed by field. Zero and false are
 * valid values; empty text is never replaced by a default.
 */
export function parseStationDraft(values) {
  const errors = {};
  const settings = {};
  if (typeof values.id !== "string" || !stationIdPattern.test(values.id)) {
    errors.id = "ID must be 1-64 ASCII letters, digits, hyphens or underscores";
  } else {
    settings.id = values.id;
  }
  if (values.enabled === "true" || values.enabled === "false") {
    settings.enabled = values.enabled === "true";
  } else {
    errors.enabled = "Choose whether the station is enabled";
  }
  for (const field of stationFields) {
    const text = String(values[field.key] ?? "").trim();
    if (text === "") {
      errors[field.key] = `${field.label} is required`;
      continue;
    }
    const value = Number(text);
    if (!numberPattern.test(text) || !Number.isFinite(value)) {
      errors[field.key] = `${field.label} must be a finite decimal number`;
      continue;
    }
    if (value < field.min || value > field.max) {
      errors[field.key] = `${field.label} must be within ${field.min}..${field.max} ${field.unit}`;
      continue;
    }
    settings[field.key] = value;
  }
  return Object.keys(errors).length > 0 ? { errors } : { settings };
}

/** Extracts the complete settings of a wire station. */
export function settingsOf(station) {
  const settings = { id: station.id, enabled: station.enabled };
  for (const field of stationFields) {
    settings[field.key] = station[field.key];
  }
  return settings;
}

function textValues(settings) {
  const values = { id: settings.id, enabled: String(settings.enabled) };
  for (const field of stationFields) {
    values[field.key] = String(settings[field.key]);
  }
  return values;
}

function formatRadius(value) {
  return `${value.toFixed(1)} NM`;
}

/**
 * Mounts the editor into panel. host supplies the shared lifecycle, the
 * bounded request function, the current submittable run (null while stale
 * or under review), the published station limit and refreshNow, which asks
 * the manager for a coherent refresh.
 */
export function mountStationEditor(panel, host) {
  const { lifecycle, request } = host;
  const list = h("div", { class: "tb-stations" });
  const listStatus = h("p", { class: "tb-muted" });
  const listNotice = h("p", { class: "tb-status", role: "status", "aria-live": "polite" });
  const listError = h("div", { class: "tb-error", role: "alert", hidden: true });
  const addButton = h("button", { type: "button" }, "Add station");
  panel.append(
    h("h2", {}, "Stations"),
    h("p", { class: "tb-muted" },
      "Coverage values are synthetic simulator reception-model estimates at the reference pressure altitude, not measured radio coverage."),
    listStatus, listNotice, listError, list, h("div", { class: "tb-actions" }, addButton));

  const state = {
    runId: null,
    server: new Map(),
    forms: new Map(),
    order: [],
    nextNew: 1,
  };

  function currentRun() {
    return host.currentRun();
  }

  // createForm builds one persistent form. Its inputs are created once so
  // polling never replaces focused elements or typed text.
  function createForm(key, mode, stationId) {
    const idPrefix = uniqueId("tb-station");
    const inputs = {};
    const errors = {};
    const labelled = (field, label, unit, control) => {
      const id = `${idPrefix}-${field}`;
      control.id = id;
      control.setAttribute("aria-describedby", `${id}-error`);
      errors[field] = h("span", { id: `${id}-error`, class: "tb-field-error" });
      inputs[field] = control;
      return h("label", { for: id }, label, unit ? h("span", { class: "tb-unit" }, unit) : null, control, errors[field]);
    };
    const idControl = h("input", { type: "text", autocomplete: "off", spellcheck: "false" });
    if (mode === "edit") {
      idControl.readOnly = true;
      idControl.value = stationId;
    }
    const enabled = h("select", {}, h("option", { value: "" }, "Choose..."),
      h("option", { value: "true" }, "Enabled"), h("option", { value: "false" }, "Disabled"));
    const fields = [
      labelled("id", "Station ID", mode === "edit" ? "immutable" : "letters, digits, - or _", idControl),
      labelled("enabled", "Reception", "", enabled),
      ...stationFields.map((field) => labelled(field.key, field.label, field.unit,
        h("input", { type: "text", inputmode: "decimal", autocomplete: "off" }))),
    ];
    const submit = h("button", { type: "submit" }, mode === "edit" ? "Save changes" : "Create station");
    const toggle = h("button", { type: "button", hidden: mode !== "edit" }, "Disable");
    const remove = h("button", { type: "button", hidden: mode !== "edit" }, mode === "edit" ? "Remove" : "Discard");
    const discard = h("button", { type: "button", hidden: mode === "edit" }, "Discard");
    const reload = h("button", { type: "button" }, "Reload server values");
    const reapply = h("button", { type: "button" }, "Reapply draft to current revision");
    const summary = h("p", { class: "tb-muted" });
    const coverage = h("p", { class: "tb-muted" });
    // The notice keeps persistent children: rebuilding buttons during a
    // click (a blur fires "change" and re-renders) would swallow the click.
    const noticeText = h("p");
    const notice = h("div", { class: "tb-notice", hidden: true }, noticeText, h("div", { class: "tb-actions" }, reload, reapply));
    const comparison = h("div", { class: "tb-scroll", hidden: true });
    const status = h("p", { class: "tb-status", role: "status", "aria-live": "polite" });
    const error = h("div", { class: "tb-error", role: "alert", hidden: true });
    const dirtyNote = h("p", { class: "tb-dirty", hidden: true }, "Unsaved draft");
    const heading = h("h3", {}, mode === "edit" ? `Station ${stationId}` : "New station");
    const formElement = h("form", { novalidate: true }, h("div", { class: "tb-fields" }, ...fields),
      h("div", { class: "tb-actions" }, submit, toggle, remove, discard));
    const element = h("article", { class: "tb-panel tb-station", "aria-label": mode === "edit" ? `Station ${stationId}` : "New station" },
      heading, summary, coverage, dirtyNote, notice, comparison, formElement, status, error);

    const form = {
      key, mode, stationId, element, inputs, errors, submit, toggle, remove, reload, reapply,
      summary, coverage, notice, noticeText, comparison, status, error, dirtyNote,
      runId: null, baseRevision: null, baseSettings: null,
      dirty: false, pending: false, attention: null,
    };

    for (const control of Object.values(inputs)) {
      lifecycle.listen(control, "input", () => {
        form.dirty = true;
        render();
      });
      lifecycle.listen(control, "change", () => {
        form.dirty = true;
        render();
      });
    }
    lifecycle.listen(formElement, "submit", (event) => {
      event.preventDefault();
      submitForm(form);
    });
    lifecycle.listen(toggle, "click", () => toggleEnabled(form));
    lifecycle.listen(remove, "click", () => removeStation(form));
    lifecycle.listen(discard, "click", () => dropForm(form));
    lifecycle.listen(reload, "click", () => reloadServerValues(form));
    lifecycle.listen(reapply, "click", () => reapplyDraft(form));
    return form;
  }

  function dropForm(form) {
    state.forms.delete(form.key);
    state.order = state.order.filter((key) => key !== form.key);
    form.element.remove();
    render();
  }

  function fillFromServer(form, server) {
    const values = textValues(settingsOf(server.station));
    for (const [key, control] of Object.entries(form.inputs)) {
      control.value = values[key];
      setText(form.errors[key], "");
    }
    form.runId = state.runId;
    form.baseRevision = server.station.revision;
    form.baseSettings = settingsOf(server.station);
    form.dirty = false;
    form.attention = null;
  }

  function draftValues(form) {
    const values = {};
    for (const [key, control] of Object.entries(form.inputs)) {
      values[key] = control.value;
    }
    return values;
  }

  function showFieldErrors(form, errors) {
    for (const [key, node] of Object.entries(form.errors)) {
      setText(node, errors[key] ?? "");
    }
  }

  // Reconciles server stations with forms. Clean edit forms follow the
  // server; dirty ones keep their draft and are flagged when the server
  // revision moved or the station disappeared.
  function reconcile() {
    for (const [id, server] of state.server) {
      if (!state.forms.has(id)) {
        const form = createForm(id, "edit", id);
        state.forms.set(id, form);
        state.order.push(id);
        fillFromServer(form, server);
        continue;
      }
      const form = state.forms.get(id);
      if (form.pending) {
        continue;
      }
      const moved = form.baseRevision !== server.station.revision || form.runId !== state.runId;
      if (!moved) {
        continue;
      }
      if (!form.dirty) {
        fillFromServer(form, server);
      } else if (form.attention !== "conflict") {
        form.attention = form.runId !== state.runId ? "run" : "changed";
      }
    }
    for (const form of [...state.forms.values()]) {
      if (form.mode !== "edit" || state.server.has(form.stationId) || form.pending) {
        continue;
      }
      if (form.dirty) {
        form.attention = "removed";
      } else {
        dropForm(form);
      }
    }
  }

  function comparisonTable(form, server) {
    const draft = draftValues(form);
    const serverValues = textValues(settingsOf(server.station));
    const rows = [["Reception", "enabled"], ...stationFields.map((field) => [`${field.label} (${field.unit})`, field.key])]
      .map(([label, key]) => h("tr", { class: draft[key] !== serverValues[key] ? "tb-changed" : null },
        h("th", { scope: "row" }, label), h("td", {}, serverValues[key]), h("td", {}, draft[key])));
    return h("table", {}, h("thead", {}, h("tr", {}, h("th", { scope: "col" }, "Setting"),
      h("th", { scope: "col" }, `Server (revision ${server.station.revision})`), h("th", { scope: "col" }, "Your draft"))),
    h("tbody", {}, ...rows));
  }

  function renderForm(form) {
    const run = currentRun();
    const server = form.mode === "edit" ? state.server.get(form.stationId) : null;
    if (server) {
      const station = server.station;
      const cov = server.coverage;
      setText(form.summary, `${station.enabled ? "Enabled" : "Disabled"}; revision ${station.revision}; created at ${station.createdAt} (virtual time).`);
      setText(form.coverage,
        `Synthetic coverage at reference pressure altitude ${cov.referenceAltitudeFeet} ft: effective radius ${formatRadius(cov.effectiveRadiusNauticalMiles)} ` +
        `(horizon ${formatRadius(cov.horizonRadiusNauticalMiles)}, link budget ${formatRadius(cov.linkBudgetRadiusNauticalMiles)}). ` +
        "Simulator reception-model estimate, not measured radio coverage.");
      setText(form.toggle, station.enabled ? "Disable" : "Enable");
    }
    form.dirtyNote.hidden = !form.dirty;

    const messages = {
      conflict: "The server rejected this change because the station changed (revision conflict). Your draft is kept.",
      changed: "The station changed on the server while you were editing. Your draft is kept.",
      run: "The simulator run changed. This draft was written for the previous run and is kept.",
      removed: `Station ${form.stationId} was removed. Stations cannot be recreated under a used ID in the same run; create a new station with a new ID.`,
    };
    if (form.attention) {
      const recoverable = form.attention !== "removed" && Boolean(server);
      setText(form.noticeText, messages[form.attention]);
      form.reload.hidden = !recoverable;
      form.reapply.hidden = !recoverable;
      form.notice.hidden = false;
      if (server && form.attention !== "removed") {
        replaceChildren(form.comparison, comparisonTable(form, server));
        form.comparison.hidden = false;
      } else {
        form.comparison.hidden = true;
      }
    } else {
      form.notice.hidden = true;
      form.comparison.hidden = true;
    }

    const blocked = run === null || form.pending || form.attention !== null;
    form.submit.disabled = blocked;
    form.toggle.disabled = blocked || !server || form.dirty;
    form.remove.disabled = blocked || !server;
    form.reapply.disabled = run === null || form.pending;
    form.reload.disabled = form.pending;
  }

  function render() {
    for (const key of state.order) {
      const form = state.forms.get(key);
      renderForm(form);
      if (form.element.parentNode !== list) {
        list.append(form.element);
      }
    }
    const count = state.server.size;
    setText(listStatus, count === 0 ? "No stations." : `${count} station${count === 1 ? "" : "s"} of at most ${host.maxStations()}.`);
    addButton.disabled = currentRun() === null;
  }

  function reloadServerValues(form) {
    const server = state.server.get(form.stationId);
    if (server) {
      fillFromServer(form, server);
      showError(form.error, null);
      setText(form.status, `Loaded server values at revision ${server.station.revision}.`);
      render();
    }
  }

  function reapplyDraft(form) {
    const server = state.server.get(form.stationId);
    if (!server || currentRun() === null) {
      return;
    }
    // The draft now targets the current revision. The user reviewed the
    // comparison and must submit again; nothing is sent here.
    form.runId = state.runId;
    form.baseRevision = server.station.revision;
    form.attention = null;
    showError(form.error, null);
    setText(form.status, `Draft now targets revision ${server.station.revision}. Review it and submit again.`);
    render();
  }

  function failureMessage(form, result) {
    const error = result.error;
    if (error.code === "conflict" && error.field === "$.runId") {
      return "The simulator run changed; this command was not applied. The new run is being loaded.";
    }
    if (error.code === "conflict") {
      return null;
    }
    if (error.code === "not_found") {
      return `Station ${form.stationId} no longer exists; it was removed by another client.`;
    }
    if (error.code === "limit") {
      return `The station limit was reached (at most ${host.maxStations()} active stations). ${error.message}`;
    }
    if (error.code === "invalid" && /already used/.test(error.message)) {
      return `${error.message}. Station IDs cannot be reused within one run, even after removal; choose a new ID.`;
    }
    return null;
  }

  // send issues one station command and handles its outcome. It never
  // retries; conflicts keep the draft and refresh the server view.
  async function send(form, method, path, body, describe) {
    form.pending = true;
    showError(form.error, null);
    setText(form.status, `Sending ${describe}...`);
    render();
    const result = await request(path, { method, body });
    if (lifecycle.destroyed) {
      return null;
    }
    form.pending = false;
    if (result.ok) {
      setText(form.status, "");
      host.refreshNow();
      return result;
    }
    setText(form.status, "");
    if (result.error.kind !== "http") {
      showError(form.error, { message: `the outcome of ${describe} is unknown (${result.error.message}). Check the refreshed stations and submit again if needed.` });
    } else if (result.error.code === "conflict" && result.error.field !== "$.runId") {
      form.attention = "conflict";
      showError(form.error, result.error, `${describe} was rejected`);
    } else {
      const message = failureMessage(form, result);
      showError(form.error, message ? { ...result.error, message } : result.error, `${describe} was rejected`);
      if (result.error.field) {
        const key = result.error.field.replace(/^\$\.station\./, "");
        if (form.errors[key]) {
          setText(form.errors[key], result.error.message);
        }
      }
    }
    host.refreshNow();
    render();
    return result;
  }

  async function submitForm(form) {
    const run = currentRun();
    if (form.pending || run === null || form.attention !== null) {
      return;
    }
    const parsed = parseStationDraft(draftValues(form));
    if (parsed.errors) {
      showFieldErrors(form, parsed.errors);
      return;
    }
    showFieldErrors(form, {});
    if (form.mode === "create") {
      const result = await send(form, "POST", "stations", { runId: run, station: parsed.settings }, `station ${parsed.settings.id} creation`);
      if (result?.ok) {
        setText(listNotice, `Station ${result.body.station.id} created at revision ${result.body.station.revision}.`);
        dropForm(form);
      }
      return;
    }
    if (form.runId !== run) {
      form.attention = "run";
      render();
      return;
    }
    const result = await send(form, "PUT", stationPath(form.stationId),
      { runId: run, expectedRevision: form.baseRevision, station: parsed.settings }, `station ${form.stationId} update`);
    if (result?.ok) {
      acknowledge(form, result.body.station);
    }
  }

  function acknowledge(form, station) {
    const server = state.server.get(form.stationId);
    state.server.set(form.stationId, { station, coverage: server?.coverage ?? null });
    if (server) {
      fillFromServer(form, state.server.get(form.stationId));
    }
    setText(form.status, `Accepted at revision ${station.revision}.`);
    render();
  }

  async function toggleEnabled(form) {
    const run = currentRun();
    const server = state.server.get(form.stationId);
    if (!server || run === null || form.pending || form.dirty || form.runId !== run) {
      return;
    }
    const settings = { ...settingsOf(server.station), enabled: !server.station.enabled };
    const result = await send(form, "PUT", stationPath(form.stationId),
      { runId: run, expectedRevision: server.station.revision, station: settings },
      `station ${form.stationId} ${settings.enabled ? "enable" : "disable"}`);
    if (result?.ok) {
      acknowledge(form, result.body.station);
    }
  }

  async function removeStation(form) {
    const run = currentRun();
    const server = state.server.get(form.stationId);
    if (!server || run === null || form.pending || form.runId !== run) {
      return;
    }
    const result = await send(form, "DELETE", stationPath(form.stationId),
      { runId: run, expectedRevision: form.baseRevision }, `station ${form.stationId} removal`);
    if (result?.ok) {
      state.server.delete(form.stationId);
      setText(listNotice, `Station ${form.stationId} removed.`);
      dropForm(form);
    }
  }

  lifecycle.listen(addButton, "click", () => {
    const key = `new-${state.nextNew++}`;
    const form = createForm(key, "create", null);
    form.runId = state.runId;
    state.forms.set(key, form);
    state.order.push(key);
    render();
    form.inputs.id.focus();
  });

  return {
    render,
    /** Reads GET stations and accepts them only for the confirmed run. */
    async refresh(signal) {
      const result = await request("stations", { signal });
      if (lifecycle.destroyed || signal.aborted) {
        return;
      }
      if (!result.ok) {
        showError(listError, result.error, "Reading stations failed");
        return;
      }
      if (result.body.runId !== state.runId) {
        // Stations and truth are separate reads. Never mix runs: wait for
        // a coherent refresh instead.
        showError(listError, { message: "stations were read from a different run; refreshing again" }, "Reading stations failed");
        host.refreshNow();
        return;
      }
      showError(listError, null);
      state.server = new Map(result.body.stations.map((entry) => [entry.station.id, entry]));
      reconcile();
    },
    hasDirtyDrafts() {
      return [...state.forms.values()].some((form) => form.dirty);
    },
    /**
     * Adopts the run confirmed by the manager. A replacement run clears
     * the previous run's server view; dirty edit drafts stay and are
     * flagged, clean ones follow the new run on the next refresh.
     */
    useRun(runId) {
      if (state.runId !== null && state.runId !== runId) {
        state.server = new Map();
        for (const form of state.forms.values()) {
          if (form.dirty && form.mode === "edit") {
            form.attention = "run";
          }
        }
      }
      state.runId = runId;
    },
  };
}
