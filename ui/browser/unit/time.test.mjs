// Unit tests for the exact decimal and virtual-time helpers.
import assert from "node:assert/strict";
import { test } from "node:test";

import {
  formatHundredths, formatNanoseconds, formatTimestamp, MAX_UINT64,
  parseDecimal, parsePositiveDecimal, parseTimestamp,
} from "../../internal/assets/static/time.js";

test("parseDecimal keeps values beyond 2^53 exact", () => {
  assert.equal(parseDecimal("0"), 0n);
  assert.equal(parseDecimal("9007199254740993"), 9007199254740993n);
  assert.equal(parseDecimal("18446744073709551615"), MAX_UINT64);
  assert.ok(parseDecimal("9007199254740993") > parseDecimal("9007199254740992"));
});

test("parseDecimal rejects non-canonical text", () => {
  for (const text of ["", "01", "-1", "+1", "1.0", "1e3", " 1", "18446744073709551616", "0x10"]) {
    assert.throws(() => parseDecimal(text), TypeError, text);
  }
  assert.throws(() => parseDecimal(1), TypeError);
  assert.throws(() => parseDecimal(null), TypeError);
  assert.throws(() => parsePositiveDecimal("0"), TypeError);
});

test("parseTimestamp is exact across the full year range", () => {
  assert.equal(parseTimestamp("1970-01-01T00:00:00Z"), 0n);
  assert.equal(parseTimestamp("1970-01-01T00:00:00.000000001Z"), 1n);
  assert.equal(parseTimestamp("1969-12-31T23:59:59.999999999Z"), -1n);
  assert.equal(parseTimestamp("0001-01-01T00:00:00Z"), -62135596800n * 1_000_000_000n);
  assert.equal(parseTimestamp("9999-12-31T23:59:59.999999999Z"), 253402300799999999999n);
  // Year 0099 must not be interpreted as 1999, as Date.UTC would.
  assert.equal(parseTimestamp("0099-03-01T00:00:00Z"), -59037897600n * 1_000_000_000n);
  assert.equal(parseTimestamp("2024-02-29T12:00:00.5Z") - parseTimestamp("2024-02-28T12:00:00Z"),
    86_400_500_000_000n);
});

test("parseTimestamp rejects invalid or non-UTC instants", () => {
  for (const text of [
    "2023-02-29T00:00:00Z", "1900-02-29T00:00:00Z", "2024-13-01T00:00:00Z", "2024-04-31T00:00:00Z",
    "2024-01-01T24:00:00Z", "2024-01-01T00:60:00Z", "2024-01-01T00:00:60Z", "0000-01-01T00:00:00Z",
    "2024-01-01T00:00:00+01:00", "2024-01-01T00:00:00", "2024-01-01T00:00:00.Z",
    "2024-01-01T00:00:00.1234567891Z", "2024-01-01 00:00:00Z", "2024-1-01T00:00:00Z",
  ]) {
    assert.throws(() => parseTimestamp(text), TypeError, text);
  }
  assert.ok(parseTimestamp("2000-02-29T00:00:00Z") > 0n, "2000 is a leap year");
});

test("formatTimestamp inverts parseTimestamp", () => {
  for (const text of [
    "0001-01-01T00:00:00Z", "0099-12-31T23:59:59.9Z", "1969-12-31T23:59:59.999999999Z",
    "1970-01-01T00:00:00Z", "2024-02-29T12:34:56.000000001Z", "9999-12-31T23:59:59.999999999Z",
  ]) {
    assert.equal(formatTimestamp(parseTimestamp(text)), text);
  }
});

test("expiry boundaries compare exactly", () => {
  const observed = parseTimestamp("2024-03-05T12:00:00Z");
  const lifetime = 30_000_000_000n;
  const atBoundary = parseTimestamp("2024-03-05T12:00:30Z");
  const pastBoundary = parseTimestamp("2024-03-05T12:00:30.000000001Z");
  assert.ok(atBoundary - observed <= lifetime);
  assert.ok(pastBoundary - observed > lifetime);
});

test("formatNanoseconds renders exact seconds with zero as a value", () => {
  assert.equal(formatNanoseconds(0n), "0 s");
  assert.equal(formatNanoseconds(1n), "0.000000001 s");
  assert.equal(formatNanoseconds(1_500_000_000n), "1.5 s");
  assert.equal(formatNanoseconds(-2_000_000_000n), "-2 s");
  assert.equal(formatNanoseconds(9007199254740993n), "9007199.254740993 s");
});

test("formatHundredths avoids floating point drift", () => {
  assert.equal(formatHundredths(0), "0x");
  assert.equal(formatHundredths(1), "0.01x");
  assert.equal(formatHundredths(10), "0.1x");
  assert.equal(formatHundredths(100), "1x");
  assert.equal(formatHundredths(125), "1.25x");
  assert.equal(formatHundredths(30), "0.3x");
  assert.equal(formatHundredths(100000), "1000x");
  assert.throws(() => formatHundredths(-1), TypeError);
  assert.throws(() => formatHundredths(1.5), TypeError);
});
