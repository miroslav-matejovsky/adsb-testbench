// Unit tests for inspector history merging and page validation. The
// inspector module imports only DOM helpers at call time, so it loads in Node.
import assert from "node:assert/strict";
import { test } from "node:test";

import { mergePage, validatePage } from "../../internal/assets/static/inspector.js";
import { at, frames, isoAt, reception } from "../fixtures.mjs";

function page(sequences, { gap = false, hasMore = true, oldest = "1", latest = "100", runId = "run-1" } = {}) {
  const records = sequences.map((sequence) => reception({
    sequence, transmissionSequence: sequence, kind: "identification", timestamp: isoAt(at(1)), frame: frames.identification,
  }));
  const last = records.length ? records[records.length - 1].sequence : "0";
  return {
    runId, stationId: "alpha", now: isoAt(at(2)), records, oldestSequence: oldest, latestSequence: latest,
    nextCursor: { runId, stationId: "alpha", afterSequence: last }, gap, hasMore, retentionLimit: 1000,
  };
}

const empty = () => ({
  runId: "run-1", stationId: "alpha", records: [], entries: [], trimmed: 0, pages: 0, nextCursor: null,
  hasMore: false, oldestSequence: null, latestSequence: null, retentionLimit: null, firstOldest: null,
  removed: false, resetRequired: null,
});

test("pages merge in order and keep the returned cursor unchanged", () => {
  const first = page([1, 2, 3]);
  const merged = mergePage(empty(), first, 10);
  assert.deepEqual(merged.records.map((record) => record.sequence), ["1", "2", "3"]);
  assert.equal(merged.nextCursor, first.nextCursor);
  assert.equal(merged.firstOldest, "1");
  assert.equal(merged.pages, 1);
});

test("overlapping pages are deduplicated without hiding gap markers", () => {
  let history = mergePage(empty(), page([1, 2, 3]), 10);
  history = mergePage(history, page([3, 4], { gap: true, oldest: "3" }), 10);
  assert.deepEqual(history.records.map((record) => record.sequence), ["1", "2", "3", "4"]);
  assert.deepEqual(history.entries.map((entry) => entry.type), ["record", "record", "record", "gap", "record"]);
});

test("a gap without records still leaves a gap row", () => {
  const history = mergePage(mergePage(empty(), page([1]), 10), page([], { gap: true, oldest: "50", hasMore: false }), 10);
  assert.equal(history.entries.at(-1).type, "gap");
  assert.equal(history.entries.at(-1).beforeSequence, "50");
});

test("browser trimming bounds loaded records and counts what it dropped", () => {
  let history = mergePage(empty(), page([1, 2, 3]), 4);
  history = mergePage(history, page([4, 5, 6], { gap: true }), 4);
  assert.deepEqual(history.records.map((record) => record.sequence), ["3", "4", "5", "6"]);
  assert.equal(history.trimmed, 2);
  assert.equal(history.entries.filter((entry) => entry.type === "gap").length, 1);
  assert.equal(history.entries.filter((entry) => entry.type === "record").length, 4);
});

test("sequences beyond 2^53 are kept as exact strings", () => {
  const big = "9007199254740993";
  const history = mergePage(empty(), page([big]), 10);
  assert.equal(history.records[0].sequence, big);
  assert.equal(history.nextCursor.afterSequence, big);
});

test("pages from another station or with malformed cursors are rejected", () => {
  validatePage(page([1]), "alpha");
  assert.throws(() => validatePage(page([1]), "bravo"), TypeError);
  const wrongCursor = page([1]);
  wrongCursor.nextCursor.runId = "run-2";
  assert.throws(() => validatePage(wrongCursor, "alpha"), TypeError);
  const badSequence = page([1]);
  badSequence.oldestSequence = "01";
  assert.throws(() => validatePage(badSequence, "alpha"), TypeError);
  assert.throws(() => validatePage(null, "alpha"), TypeError);
});
