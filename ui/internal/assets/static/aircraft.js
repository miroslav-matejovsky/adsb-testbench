// Aircraft display component: received aircraft, station selection, map and
// reception inspector.
//
// Each refresh reads GET stations and POST observations {stationIds} from
// the display backend (never the simulator) and uses its own responses; the
// shared GET snapshot route is never read. The component retains at most the
// current successful view and the one immediately before it (for one-refresh
// tombstones). A failed refresh shows that same-selection, same-run view
// marked transport-stale with its real update age; virtual ages and track
// classes stay frozen because they only change with a new virtual snapshot.
//
// A selection change aborts the in-flight refresh and discards the old
// selection's data. A confirmed replacement run (from the catalog, a
// snapshot, or an error envelope) clears every previous-run row, cursor and
// selection target before the next coherent refresh. Station and
// observation reads are separate instants and are only combined when both
// name the same run.

import { createPoller, requestJSON } from "./client.js";
import { busyWhile, createScaffold, ensureStylesheet, showError } from "./component.js";
import { validateAircraftConfig } from "./config.js";
import { h, replaceChildren, setText } from "./dom.js";
import { clippedByProjection } from "./geo.js";
import { createInspector } from "./inspector.js";
import { createAircraftMap } from "./map.js";
import { formatNanoseconds } from "./time.js";
import { buildView, normalizeSelection, selectionKey, validateObservations } from "./tracks.js";

const statusLabels = { fresh: "Fresh", stale: "Stale", lost: "Lost" };

/** Formats a received field value with its virtual age, or "unavailable". */
function withAge(text, entry) {
  return `${text} (age ${formatNanoseconds(entry.ageNs)})`;
}

function unavailable() {
  return h("span", { class: "tb-unavailable" }, "unavailable");
}

function measurementText(measurement, unit) {
  if (!measurement) {
    return null;
  }
  const base = `${measurement.value} ${unit}`;
  return measurement.overRange ? `${base} (at or beyond the encodable limit)` : base;
}

/** Describes each received field of a row as display text, or null. */
export function describeRow(row) {
  const velocity = row.velocity;
  const ground = velocity && velocity.groundSpeedKnots !== null
    ? withAge(`${velocity.groundSpeedKnots.toFixed(1)} kt, track ${velocity.trackDegrees === null ? "unavailable" : `${velocity.trackDegrees.toFixed(1)} deg`}`, velocity)
    : null;
  const air = velocity && velocity.airspeed
    ? withAge(`${measurementText(velocity.airspeed, "kt")} ${velocity.trueAirspeed ? "true airspeed" : "indicated airspeed"}, heading ${velocity.headingDegrees === null ? "unavailable" : `${velocity.headingDegrees.toFixed(1)} deg`}`, velocity)
    : null;
  const vertical = velocity && velocity.verticalRate
    ? withAge(`${measurementText(velocity.verticalRate, "ft/min")} (${velocity.barometricVerticalRate ? "barometric" : "GNSS"})`, velocity)
    : null;
  return {
    callsign: row.identity ? withAge(row.identity.callsign, row.identity) : null,
    position: row.position
      ? withAge(`${row.position.latitudeDegrees.toFixed(5)}, ${row.position.longitudeDegrees.toFixed(5)}` +
        (clippedByProjection(row.position.latitudeDegrees) ? " (beyond the map projection limit, drawn clipped)" : ""), row.position)
      : null,
    altitude: row.altitude ? withAge(`${row.altitude.feet} ft pressure altitude`, row.altitude) : null,
    ground,
    air,
    vertical,
    lastReceived: `${row.lastReceivedAt} (age ${formatNanoseconds(row.ageNs)})`,
    receivers: row.receivers.length > 0 ? row.receivers.join(", ") : null,
  };
}

/** Mounts an aircraft display into root. See components.js. */
export function mountAircraftDisplay(root, rawConfig) {
  const config = validateAircraftConfig(rawConfig, window.location.origin);
  ensureStylesheet(config.assetBaseUrl + "leaflet/leaflet.css");
  ensureStylesheet(config.assetBaseUrl + "ui.css");
  const { lifecycle, wrapper, status, error, content } = createScaffold(root, { kind: "aircraft", label: "Received aircraft" });
  const request = (path, options = {}) => {
    const controller = lifecycle.controller();
    const forward = () => controller.abort();
    options.signal?.addEventListener("abort", forward, { once: true });
    return requestJSON(config.apiBaseUrl + path, {
      ...options, signal: controller.signal,
      timeoutMs: config.requestTimeoutMilliseconds, maxBytes: config.maxResponseBytes,
    }).finally(() => {
      options.signal?.removeEventListener("abort", forward);
      controller.release();
    });
  };

  const state = {
    runId: null,
    catalog: null,
    catalogError: null,
    selection: normalizeSelection(config.stationIds),
    view: null,
    lastSuccessAt: null,
    transportError: null,
    selectedIcao: null,
    generation: 0,
  };

  // Station selector.
  const selector = h("fieldset", { class: "tb-selector" }, h("legend", {}, "Stations"));
  const selectorOptions = h("div", { class: "tb-actions" });
  const selectorNote = h("p", { class: "tb-muted" });
  selector.append(selectorOptions, selectorNote);
  const checkboxes = new Map();

  // Transport banner, table and details.
  const banner = h("p", { class: "tb-banner", role: "status", "aria-live": "polite" });
  const virtualLine = h("p", { class: "tb-muted" });
  const emptyNote = h("p", { class: "tb-muted", hidden: true });
  const tableBody = h("tbody");
  const table = h("table", { "aria-label": "Received aircraft tracks" },
    h("thead", {}, h("tr", {}, ...["ICAO", "Track", "Callsign", "Position", "Altitude", "Ground speed / track",
      "Airspeed / heading", "Vertical rate", "Last received (virtual)", "Receivers"].map((title) => h("th", { scope: "col" }, title)))),
    tableBody);
  const tablePanel = h("section", { class: "tb-panel", "aria-label": "Received aircraft table" },
    h("h2", {}, "Received aircraft"),
    h("p", { class: "tb-muted" }, "Decoded from received frames only. Unknown values are shown as unavailable, never as zero."),
    virtualLine, emptyNote, h("div", { class: "tb-scroll" }, table));
  const details = h("dl", { class: "tb-facts" });
  const detailsPanel = h("section", { class: "tb-panel", "aria-label": "Selected aircraft", "aria-live": "polite" },
    h("h2", {}, "Selected aircraft"), details);
  const mapPanel = h("section", { class: "tb-panel", "aria-label": "Map" }, h("h2", {}, "Map"));
  const inspectorPanel = h("section", { class: "tb-panel", "aria-label": "Reception inspector" });
  content.append(h("div", { class: "tb-panel" }, selector), banner, mapPanel, tablePanel, detailsPanel, inspectorPanel);

  const map = createAircraftMap(mapPanel, {
    lifecycle, config, onSelect: (icao) => select(icao),
  });
  const inspector = createInspector(inspectorPanel, { lifecycle, config, request, currentRun: () => state.runId });
  lifecycle.onDestroy(() => map.destroy());

  function select(icao) {
    state.selectedIcao = icao;
    render();
  }

  // clearRun discards everything that belongs to a previous run.
  function adoptRun(runId) {
    if (state.runId !== null && state.runId !== runId) {
      state.view = null;
      state.lastSuccessAt = null;
      state.selectedIcao = null;
      state.catalog = null;
      inspector.reset("the simulator run changed");
    }
    state.runId = runId;
  }

  function changeSelection(selection) {
    state.selection = normalizeSelection(selection);
    state.generation += 1;
    state.view = null;
    state.lastSuccessAt = null;
    state.transportError = null;
    state.selectedIcao = null;
    render();
    poller.restart();
  }

  function renderSelector() {
    const catalog = state.catalog?.stations ?? [];
    const known = new Set(catalog.map((entry) => entry.station.id));
    const ids = [...new Set([...catalog.map((entry) => entry.station.id), ...state.selection])];
    for (const [id, entry] of checkboxes) {
      if (!ids.includes(id)) {
        entry.label.remove();
        checkboxes.delete(id);
      }
    }
    for (const id of ids) {
      let entry = checkboxes.get(id);
      if (!entry) {
        const input = h("input", { type: "checkbox", value: id });
        const text = h("span");
        const label = h("label", { class: "tb-choice" }, input, text);
        entry = { input, text, label };
        checkboxes.set(id, entry);
        lifecycle.listen(input, "change", () => {
          const next = input.checked ? [...state.selection, id] : state.selection.filter((other) => other !== id);
          changeSelection(next);
        });
      }
      const station = catalog.find((candidate) => candidate.station.id === id)?.station;
      entry.input.checked = state.selection.includes(id);
      setText(entry.text, station
        ? `${id} (${station.enabled ? "enabled" : "disabled"}, revision ${station.revision})`
        : `${id} (unavailable)`);
      selectorOptions.append(entry.label);
    }
    const missing = state.selection.filter((id) => state.catalog && !known.has(id));
    setText(selectorNote, missing.length > 0
      ? `Selected station ${missing.join(", ")} is not in the current catalog. Change the selection explicitly.`
      : state.selection.length === 0 ? "No stations selected." : "");
  }

  function renderBanner() {
    const updated = state.lastSuccessAt === null ? null
      : `${new Date(state.lastSuccessAt).toISOString()} (${Math.max(0, Math.round((Date.now() - state.lastSuccessAt) / 1000))} s ago, real time)`;
    const failure = state.transportError;
    banner.classList.toggle("tb-stale", failure !== null && state.view !== null);
    if (failure && state.view) {
      setText(banner, `Transport stale: showing the last successful data for this selection from ${updated}. The latest refresh failed.`);
    } else if (failure) {
      setText(banner, "No received data for this selection: the latest refresh failed.");
    } else if (state.view) {
      setText(banner, `Received data is current. Last successful update ${updated}.`);
    } else {
      setText(banner, "Waiting for received data...");
    }
    showError(error, failure, "Refreshing received aircraft failed");
    if (!failure && state.catalogError) {
      showError(error, state.catalogError, "Reading stations failed");
    }
  }

  function cell(text) {
    return h("td", {}, text ?? unavailable());
  }

  function renderTable() {
    const view = state.view;
    const focusedIcao = tableBody.contains(document.activeElement) ? document.activeElement.dataset.icao : null;
    if (!view) {
      setText(virtualLine, "");
      emptyNote.hidden = true;
      replaceChildren(tableBody);
      return;
    }
    setText(virtualLine, `Virtual time of this snapshot: ${view.now}. Run ${view.runId}.`);
    emptyNote.hidden = view.rows.length > 0;
    setText(emptyNote, view.selection.length === 0 ? "No stations selected, so no aircraft are received."
      : "No aircraft are received by the selected stations.");
    const rows = view.rows.map((row) => {
      const text = describeRow(row);
      const selected = row.icao === state.selectedIcao;
      const status = row.tombstone ? "Lost - No retained evidence" : statusLabels[row.status];
      const tr = h("tr", {
        class: `tb-row-selectable tb-track-${row.status}`, tabindex: "0", "data-icao": row.icao,
        "aria-selected": selected ? "true" : "false",
      },
      h("th", { scope: "row" }, row.icao), h("td", {}, status),
      row.tombstone ? cell(row.callsign) : cell(text.callsign),
      cell(text.position), cell(text.altitude), cell(text.ground), cell(text.air), cell(text.vertical),
      h("td", {}, text.lastReceived), cell(text.receivers));
      return tr;
    });
    replaceChildren(tableBody, ...rows);
    if (focusedIcao) {
      tableBody.querySelector(`tr[data-icao="${focusedIcao}"]`)?.focus();
    }
  }

  // Rows are rebuilt on every render, so selection uses one delegated
  // listener pair on the table body instead of per-row listeners.
  const rowIcao = (event) => event.target.closest?.("tr[data-icao]")?.dataset.icao ?? null;
  lifecycle.listen(tableBody, "click", (event) => {
    const icao = rowIcao(event);
    if (icao) {
      select(icao);
    }
  });
  lifecycle.listen(tableBody, "keydown", (event) => {
    const icao = rowIcao(event);
    if (icao && (event.key === "Enter" || event.key === " ")) {
      event.preventDefault();
      select(icao);
    }
  });

  function renderDetails() {
    const row = state.view?.rows.find((candidate) => candidate.icao === state.selectedIcao) ?? null;
    if (!row) {
      replaceChildren(details, h("dt", {}, "Aircraft"), h("dd", {}, "Select an aircraft in the table or on the map."));
      return;
    }
    const text = describeRow(row);
    const facts = [
      ["ICAO", row.icao],
      ["Track status", row.tombstone ? "Lost - No retained evidence" : statusLabels[row.status]],
      ["Callsign", row.tombstone ? row.callsign : text.callsign],
      ["Position", text.position],
      ["Pressure altitude", text.altitude],
      ["Ground speed / track", text.ground],
      ["Airspeed / heading", text.air],
      ["Vertical rate", text.vertical],
      ["Last received (virtual)", text.lastReceived],
      ["Receivers", text.receivers],
    ];
    replaceChildren(details, ...facts.flatMap(([term, value]) => [h("dt", {}, term), h("dd", {}, value ?? unavailable())]));
  }

  function render() {
    if (lifecycle.destroyed) {
      return;
    }
    renderSelector();
    renderBanner();
    renderTable();
    renderDetails();
    map.update({
      view: state.view, catalog: state.catalog, selection: state.selection,
      selectedIcao: state.selectedIcao, stale: state.transportError !== null,
    });
    inspector.update({ view: state.view, catalog: state.catalog, selectedIcao: state.selectedIcao });
  }

  function handleStations(result) {
    if (result.ok && typeof result.body?.runId === "string" && Array.isArray(result.body.stations)) {
      adoptRun(result.body.runId);
      state.catalog = result.body;
      state.catalogError = null;
      return;
    }
    state.catalogError = result.error ?? { message: "the station catalog is invalid" };
    if (result.error?.runId && state.runId !== null && result.error.runId !== state.runId) {
      adoptRun(result.error.runId);
    }
  }

  function handleObservations(result, selection) {
    const observations = result.body?.snapshot?.observations ?? null;
    if (!result.ok) {
      state.transportError = result.error;
      // A validated envelope naming another run confirms a replacement,
      // even when that run's first payload was unusable.
      const runId = result.error.runId;
      if (runId && state.runId !== null && runId !== state.runId) {
        adoptRun(runId);
        poller.refreshNow();
      }
      return;
    }
    try {
      validateObservations(observations, selection);
    } catch (problem) {
      state.transportError = { kind: "invalid-response", message: `invalid observations: ${problem.message}` };
      return;
    }
    if (state.catalog && state.catalog.runId !== observations.runId) {
      // Separate reads from different runs: never combine them.
      adoptRun(observations.runId);
      state.transportError = { kind: "invalid-response", message: "the simulator run changed during the refresh; refreshing again" };
      poller.refreshNow();
      return;
    }
    adoptRun(observations.runId);
    const previous = state.view && state.view.runId === observations.runId &&
      selectionKey(state.view.selection) === selectionKey(selection) ? state.view : null;
    try {
      state.view = buildView(observations, previous, config);
    } catch (problem) {
      state.transportError = { kind: "invalid-response", message: `invalid observations: ${problem.message}` };
      return;
    }
    state.lastSuccessAt = Date.now();
    state.transportError = null;
    setText(status, "Received data loaded.");
  }

  async function refresh(signal) {
    const generation = state.generation;
    const selection = state.selection.slice();
    const [stations, observed] = await Promise.all([
      request("stations", { signal }),
      request("observations", { method: "POST", body: { stationIds: selection }, signal }),
    ]);
    if (lifecycle.destroyed || signal.aborted || generation !== state.generation) {
      return;
    }
    handleStations(stations);
    handleObservations(observed, selection);
    render();
  }

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
