// Reception inspector for the aircraft display.
//
// Evidence: the selected aircraft's field evidence is rendered exactly as
// the backend returned it; frames are never reconstructed. A position shows
// both CPR frames. Every receiver copy is listed separately with the station
// revision and the station settings recorded at reception time, never the
// current catalog settings.
//
// History: one station's retained receptions are paged with display
// POST receptions/history {stationId, cursor, limit}. The first request uses
// cursor null; later requests send the returned nextCursor unchanged, only
// while hasMore is true. A source gap (older receptions evicted) is kept as
// a persistent row. Loaded rows are bounded by maxHistoryRecords: the
// browser trims the oldest loaded rows and says so, separately from source
// gaps. One history request is active at a time; a station, selection or
// run change invalidates it. A cursor conflict or replacement run requires
// an explicit reset; new-run records are never appended to old history.

import { h, replaceChildren, setText, uniqueId } from "./dom.js";
import { parseDecimal } from "./time.js";

/** Maximum distinct gap markers kept per loaded history. */
const MAX_GAP_ROWS = 1000;

function receiverSummary(receiver) {
  return `${receiver.enabled ? "enabled" : "disabled"}; lat ${receiver.latitudeDegrees}, lon ${receiver.longitudeDegrees} deg; ` +
    `site ${receiver.siteElevationMetres} m, antenna ${receiver.antennaHeightMetres} m; gain ${receiver.antennaGainDBi} dBi; ` +
    `sensitivity ${receiver.sensitivityDBm} dBm; loss ${receiver.systemLossDB} dB; frame loss ${receiver.frameLossProbability}`;
}

/**
 * Merges one validated page into loaded history. Returns the new history
 * state; the input is not modified. Records are deduplicated by run,
 * station and reception sequence, gap markers are kept, and the oldest
 * records are trimmed to maxRecords with the trimmed count accumulated.
 */
export function mergePage(history, page, maxRecords) {
  const known = new Set(history.records.map((record) => `${page.runId}/${record.stationId}/${record.sequence}`));
  const records = [...history.records];
  const entries = [...history.entries];
  if (page.gap) {
    entries.push({ type: "gap", beforeSequence: page.oldestSequence, key: `gap-${history.pages}` });
  }
  for (const record of page.records) {
    const key = `${page.runId}/${record.stationId}/${record.sequence}`;
    if (known.has(key)) {
      continue;
    }
    known.add(key);
    records.push(record);
    entries.push({ type: "record", record, key });
  }
  let trimmed = history.trimmed;
  const excess = records.length - maxRecords;
  if (excess > 0) {
    const dropped = new Set(records.splice(0, excess).map((record) => record.sequence));
    trimmed += excess;
    for (let index = entries.length - 1; index >= 0; index--) {
      if (entries[index].type === "record" && dropped.has(entries[index].record.sequence)) {
        entries.splice(index, 1);
      }
    }
  }
  const gaps = entries.filter((entry) => entry.type === "gap");
  if (gaps.length > MAX_GAP_ROWS) {
    entries.splice(entries.indexOf(gaps[0]), 1);
  }
  return {
    ...history,
    records, entries, trimmed,
    pages: history.pages + 1,
    nextCursor: page.nextCursor,
    hasMore: page.hasMore,
    oldestSequence: page.oldestSequence,
    latestSequence: page.latestSequence,
    retentionLimit: page.retentionLimit,
    firstOldest: history.firstOldest ?? page.oldestSequence,
  };
}

/** Validates one history page against the request that produced it. */
export function validatePage(page, stationId) {
  if (!page || typeof page.runId !== "string" || page.runId === "" || page.stationId !== stationId ||
      !Array.isArray(page.records) || typeof page.gap !== "boolean" || typeof page.hasMore !== "boolean" ||
      !page.nextCursor || page.nextCursor.runId !== page.runId || page.nextCursor.stationId !== stationId) {
    throw new TypeError("the history page does not match the request");
  }
  parseDecimal(page.oldestSequence);
  parseDecimal(page.latestSequence);
  parseDecimal(page.nextCursor.afterSequence);
  for (const record of page.records) {
    parseDecimal(record.sequence);
    if (record.stationId !== stationId || typeof record.frame !== "string") {
      throw new TypeError("a history record belongs to another station");
    }
  }
}

function emptyHistory(runId, stationId) {
  return {
    runId, stationId, records: [], entries: [], trimmed: 0, pages: 0,
    nextCursor: null, hasMore: false, oldestSequence: null, latestSequence: null,
    retentionLimit: null, firstOldest: null, removed: false, resetRequired: null,
  };
}

/**
 * Creates the inspector inside panel. host: { lifecycle, config, request,
 * currentRun }. Returns { update(model), reset(reason) }.
 */
export function createInspector(panel, host) {
  const { lifecycle, config, request } = host;
  const evidenceBody = h("div", { class: "tb-evidence" });
  const copyStatus = h("p", { class: "tb-status", role: "status", "aria-live": "polite" });
  const copyError = h("div", { class: "tb-error", role: "alert", hidden: true });

  const stationSelectId = uniqueId("tb-history-station");
  const stationSelect = h("select", { id: stationSelectId });
  const loadButton = h("button", { type: "button" }, "Load history");
  const nextButton = h("button", { type: "button", disabled: true }, "Next page");
  const resetButton = h("button", { type: "button", disabled: true }, "Reset");
  const historyStatus = h("p", { class: "tb-status", role: "status", "aria-live": "polite" });
  const historyError = h("div", { class: "tb-error", role: "alert", hidden: true });
  const historyNotes = h("div", {});
  const historyBody = h("tbody");
  const historyTable = h("table", { "aria-label": "Reception history" },
    h("thead", {}, h("tr", {}, ...["Reception", "Transmission", "Virtual time", "ICAO", "Kind", "Frame", "Station revision",
      "Slant range (NM)", "Received power (dBm)", "Receiver settings at reception"].map((title) => h("th", { scope: "col" }, title)))),
    historyBody);

  panel.append(
    h("h2", {}, "Reception inspector"),
    h("section", { "aria-label": "Field evidence" }, h("h3", {}, "Field evidence"), copyStatus, copyError, evidenceBody),
    h("section", { "aria-label": "Station reception history" },
      h("h3", {}, "Station reception history"),
      h("div", { class: "tb-actions" }, h("label", { for: stationSelectId, class: "tb-choice" }, "Station", stationSelect),
        loadButton, nextButton, resetButton),
      historyStatus, historyError, historyNotes, h("div", { class: "tb-scroll" }, historyTable)));

  const state = {
    evidenceKey: null,
    catalog: null,
    history: null,
    pending: false,
    generation: 0,
  };

  // copyFrame copies exact frame text; a denied or missing clipboard is
  // reported instead of failing silently.
  async function copyFrame(frame) {
    showCopyError(null);
    try {
      if (!navigator.clipboard?.writeText) {
        throw new Error("the clipboard is not available in this browser context");
      }
      await navigator.clipboard.writeText(frame);
      if (!lifecycle.destroyed) {
        setText(copyStatus, `Copied frame ${frame}.`);
      }
    } catch (error) {
      if (!lifecycle.destroyed) {
        setText(copyStatus, "");
        showCopyError(`Copying frame ${frame} failed: ${error.message}`);
      }
    }
  }

  function showCopyError(message) {
    copyError.hidden = message === null;
    setText(copyError, message ?? "");
  }

  function frameCell(frame) {
    return h("td", {}, h("span", { class: "tb-frame" }, frame), " ",
      h("button", { type: "button", class: "tb-copy", "data-frame": frame, "aria-label": `Copy frame ${frame}` }, "Copy"));
  }

  lifecycle.listen(panel, "click", (event) => {
    const button = event.target.closest?.("button.tb-copy");
    if (button && panel.contains(button)) {
      copyFrame(button.dataset.frame);
    }
  });

  function receptionTable(receptions) {
    return h("table", { class: "tb-receptions" },
      h("thead", {}, h("tr", {}, ...["Station", "Station revision", "Reception", "Transmission", "Slant range (NM)",
        "Received power (dBm)", "Receiver settings at reception"].map((title) => h("th", { scope: "col" }, title)))),
      h("tbody", {}, ...receptions.map((copy) => h("tr", {},
        h("td", {}, copy.stationId), h("td", {}, copy.stationRevision), h("td", {}, copy.sequence),
        h("td", {}, copy.transmissionSequence), h("td", {}, String(copy.slantRangeNauticalMiles)),
        h("td", {}, String(copy.receivedPowerDBm)), h("td", {}, receiverSummary(copy.receiver))))));
  }

  function evidenceBlock(title, evidence) {
    return h("div", { class: "tb-evidence-item", "data-transmission": evidence.transmissionSequence },
      h("h4", {}, title),
      h("dl", { class: "tb-facts" },
        h("dt", {}, "Transmission sequence"), h("dd", {}, evidence.transmissionSequence),
        h("dt", {}, "ICAO"), h("dd", {}, evidence.icao),
        h("dt", {}, "Kind"), h("dd", {}, evidence.kind),
        h("dt", {}, "Virtual time"), h("dd", {}, evidence.timestamp),
        h("dt", {}, "Frame"), h("dd", {}, h("span", { class: "tb-frame" }, evidence.frame), " ",
          h("button", { type: "button", class: "tb-copy", "data-frame": evidence.frame, "aria-label": `Copy frame ${evidence.frame}` }, "Copy"))),
      h("div", { class: "tb-scroll" }, receptionTable(evidence.receptions)));
  }

  function renderEvidence(view, selectedIcao) {
    const row = view?.rows.find((candidate) => candidate.icao === selectedIcao) ?? null;
    const key = row && row.source ? `${view.runId}/${view.now}/${row.icao}` : `none/${selectedIcao}/${row?.tombstone}`;
    if (key === state.evidenceKey) {
      return;
    }
    state.evidenceKey = key;
    if (!row) {
      replaceChildren(evidenceBody, h("p", { class: "tb-muted" }, "Select an aircraft to inspect the frames behind its received fields."));
      return;
    }
    if (!row.source) {
      replaceChildren(evidenceBody, h("p", { class: "tb-muted" }, `${row.icao}: no retained evidence.`));
      return;
    }
    const source = row.source;
    const blocks = [];
    if (source.identity) {
      blocks.push(evidenceBlock("Identity", source.identity.evidence));
    }
    if (source.position) {
      source.position.evidence.forEach((evidence, index) => {
        blocks.push(evidenceBlock(`Position CPR frame ${index + 1} of ${source.position.evidence.length}`, evidence));
      });
    }
    if (source.barometricAltitude) {
      blocks.push(evidenceBlock("Pressure altitude", source.barometricAltitude.evidence));
    }
    if (source.velocity) {
      blocks.push(evidenceBlock("Velocity", source.velocity.evidence));
    }
    replaceChildren(evidenceBody, h("p", { class: "tb-muted" },
      `Received evidence for ${row.icao} at virtual time ${view.now}. Generated simulator frames are shown separately in the manager.`),
    ...(blocks.length > 0 ? blocks : [h("p", { class: "tb-muted" }, "No field evidence is retained.")]));
  }

  function renderStations() {
    const current = stationSelect.value;
    const ids = (state.catalog?.stations ?? []).map((entry) => entry.station.id);
    const options = ids.map((id) => h("option", { value: id }, id));
    if (state.history?.removed && !ids.includes(state.history.stationId)) {
      options.push(h("option", { value: state.history.stationId, disabled: true }, `${state.history.stationId} (removed)`));
    }
    const signature = JSON.stringify(ids) + (state.history?.removed ? state.history.stationId : "");
    if (stationSelect.dataset.signature !== signature) {
      replaceChildren(stationSelect, h("option", { value: "" }, "Choose a station"), ...options);
      stationSelect.dataset.signature = signature;
      stationSelect.value = ids.includes(current) ? current : "";
    }
  }

  function renderHistory() {
    const history = state.history;
    const available = stationSelect.value !== "" && host.currentRun() !== null;
    loadButton.disabled = state.pending || !available;
    nextButton.disabled = state.pending || !history || !history.hasMore || history.removed || history.resetRequired !== null;
    resetButton.disabled = state.pending || !history;
    if (!history) {
      replaceChildren(historyNotes);
      replaceChildren(historyBody);
      return;
    }
    const notes = [];
    if (history.pages > 0) {
      notes.push(h("p", { class: "tb-muted" },
        `Station ${history.stationId}, run ${history.runId}: retained sequences ${history.oldestSequence}..${history.latestSequence} ` +
        `(source retention limit ${history.retentionLimit}). Loaded ${history.records.length} of at most ${config.maxHistoryRecords} rows. ` +
        (history.hasMore ? "More records are available." : "No more records at the time of the last page; use Reset to refresh.")));
    }
    if (history.firstOldest !== null && parseDecimal(history.firstOldest) > 1n) {
      notes.push(h("p", { class: "tb-notice" },
        `Receptions before sequence ${history.firstOldest} of this run are outside source retention.`));
    }
    if (history.trimmed > 0) {
      notes.push(h("p", { class: "tb-notice" },
        `Browser trimmed ${history.trimmed} older loaded row${history.trimmed === 1 ? "" : "s"} to stay within ${config.maxHistoryRecords} rows. ` +
        "This is a display limit, not a source gap."));
    }
    if (history.removed) {
      notes.push(h("p", { class: "tb-notice" },
        `Station ${history.stationId} was removed. The loaded rows are historical; choose an available station for new requests.`));
    }
    if (history.resetRequired) {
      notes.push(h("p", { class: "tb-notice" }, `Reset required: ${history.resetRequired}`));
    }
    replaceChildren(historyNotes, ...notes);
    replaceChildren(historyBody, ...history.entries.map((entry) => {
      if (entry.type === "gap") {
        return h("tr", { class: "tb-gap-row" }, h("td", { colspan: "10" },
          `Gap: older receptions were evicted by source retention before sequence ${entry.beforeSequence}. History is not contiguous here.`));
      }
      const record = entry.record;
      return h("tr", { "data-sequence": record.sequence },
        h("td", {}, record.sequence), h("td", {}, record.transmissionSequence), h("td", {}, record.timestamp),
        h("td", {}, record.icao), h("td", {}, record.kind), frameCell(record.frame), h("td", {}, record.stationRevision),
        h("td", {}, String(record.slantRangeNauticalMiles)), h("td", {}, String(record.receivedPowerDBm)),
        h("td", {}, receiverSummary(record.receiver)));
    }));
  }

  function showHistoryError(message) {
    historyError.hidden = message === null;
    setText(historyError, message ?? "");
  }

  function invalidateHistory() {
    state.generation += 1;
    state.pending = false;
  }

  async function loadPage(stationId, cursor) {
    const runId = host.currentRun();
    if (state.pending || runId === null) {
      return;
    }
    const generation = ++state.generation;
    state.pending = true;
    showHistoryError(null);
    setText(historyStatus, `Loading reception history for ${stationId}...`);
    renderHistory();
    const result = await request("receptions/history", {
      method: "POST", body: { stationId, cursor, limit: config.historyPageSize },
    });
    if (lifecycle.destroyed || generation !== state.generation) {
      return;
    }
    state.pending = false;
    setText(historyStatus, "");
    if (!result.ok) {
      if (result.error.code === "conflict") {
        state.history.resetRequired = "the cursor belongs to another run. Reset and load again.";
      } else if (result.error.code === "not_found") {
        state.history.removed = true;
      }
      showHistoryError(`Loading history failed: ${result.error.message}${result.error.code ? ` (code ${result.error.code})` : ""}`);
      renderHistory();
      return;
    }
    try {
      validatePage(result.body, stationId);
    } catch (problem) {
      showHistoryError(`Loading history failed: ${problem.message}`);
      renderHistory();
      return;
    }
    if (result.body.runId !== state.history.runId) {
      state.history.resetRequired = `the simulator run changed to ${result.body.runId}. Reset and load again.`;
      renderHistory();
      return;
    }
    state.history = mergePage(state.history, result.body, config.maxHistoryRecords);
    renderHistory();
  }

  lifecycle.listen(loadButton, "click", () => {
    const stationId = stationSelect.value;
    if (stationId === "" || host.currentRun() === null) {
      return;
    }
    invalidateHistory();
    state.history = emptyHistory(host.currentRun(), stationId);
    loadPage(stationId, null);
  });
  lifecycle.listen(nextButton, "click", () => {
    const history = state.history;
    if (history && history.hasMore && !history.removed && history.resetRequired === null) {
      loadPage(history.stationId, history.nextCursor);
    }
  });
  lifecycle.listen(resetButton, "click", () => {
    invalidateHistory();
    state.history = null;
    showHistoryError(null);
    setText(historyStatus, "History cleared.");
    renderHistory();
  });
  lifecycle.listen(stationSelect, "change", () => {
    invalidateHistory();
    state.history = null;
    showHistoryError(null);
    setText(historyStatus, "");
    renderHistory();
  });

  return {
    update({ view, catalog, selectedIcao }) {
      if (lifecycle.destroyed) {
        return;
      }
      state.catalog = catalog;
      if (state.history && catalog && catalog.runId === state.history.runId &&
          !catalog.stations.some((entry) => entry.station.id === state.history.stationId)) {
        state.history.removed = true;
      }
      renderEvidence(view, selectedIcao);
      renderStations();
      renderHistory();
    },
    /** Clears evidence, cursors and loaded rows, for example on a run change. */
    reset(reason) {
      invalidateHistory();
      state.evidenceKey = null;
      if (state.history) {
        state.history = null;
        setText(historyStatus, `History cleared: ${reason}.`);
      }
      showHistoryError(null);
    },
  };
}
