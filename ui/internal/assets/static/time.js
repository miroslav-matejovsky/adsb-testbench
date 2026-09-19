// Exact decimal and virtual-time helpers.
//
// The APIs carry run revisions, sequences and nanosecond durations as
// canonical decimal strings, and instants as UTC RFC3339Nano text. Browser
// numbers cannot hold those values exactly, so they are parsed into BigInt:
// decimals as-is, instants as signed nanoseconds since 1970-01-01T00:00:00Z.
// Comparisons and differences stay exact over the full year range 0001-9999.
// Only presentation (formatNanoseconds, formatHundredths) produces text.

const decimalPattern = /^(0|[1-9][0-9]*)$/;
const timestampPattern = /^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(?:\.([0-9]{1,9}))?Z$/;

/** Largest canonical unsigned decimal accepted: 2^64 - 1. */
export const MAX_UINT64 = (1n << 64n) - 1n;

/** Nanoseconds per second as BigInt. */
export const NANOS_PER_SECOND = 1_000_000_000n;

const NANOS_PER_DAY = 86_400n * NANOS_PER_SECOND;

/**
 * Parses a canonical unsigned decimal string (no sign, no leading zero,
 * at most 2^64 - 1) into a BigInt. Throws TypeError on anything else.
 */
export function parseDecimal(text) {
  if (typeof text !== "string" || !decimalPattern.test(text)) {
    throw new TypeError(`not a canonical decimal: ${JSON.stringify(text)}`);
  }
  const value = BigInt(text);
  if (value > MAX_UINT64) {
    throw new TypeError(`decimal exceeds 2^64-1: ${text}`);
  }
  return value;
}

/** Parses a canonical positive decimal (at least 1). */
export function parsePositiveDecimal(text) {
  const value = parseDecimal(text);
  if (value === 0n) {
    throw new TypeError("decimal must be positive");
  }
  return value;
}

function isLeapYear(year) {
  return (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0;
}

function daysInMonth(year, month) {
  return [31, isLeapYear(year) ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1];
}

// daysFromCivil returns days since 1970-01-01 for a proleptic Gregorian
// date (Howard Hinnant's algorithm), exact for every year 0001-9999.
function daysFromCivil(year, month, day) {
  const y = month <= 2 ? year - 1 : year;
  const era = Math.floor(y / 400);
  const yoe = y - era * 400;
  const mp = (month + 9) % 12;
  const doy = Math.floor((153 * mp + 2) / 5) + day - 1;
  const doe = yoe * 365 + Math.floor(yoe / 4) - Math.floor(yoe / 100) + doy;
  return era * 146097 + doe - 719468;
}

function civilFromDays(days) {
  const z = days + 719468;
  const era = Math.floor(z / 146097);
  const doe = z - era * 146097;
  const yoe = Math.floor((doe - Math.floor(doe / 1460) + Math.floor(doe / 36524) - Math.floor(doe / 146096)) / 365);
  const doy = doe - (365 * yoe + Math.floor(yoe / 4) - Math.floor(yoe / 100));
  const mp = Math.floor((5 * doy + 2) / 153);
  const day = doy - Math.floor((153 * mp + 2) / 5) + 1;
  const month = mp < 10 ? mp + 3 : mp - 9;
  return { year: yoe + era * 400 + (month <= 2 ? 1 : 0), month, day };
}

/**
 * Parses a UTC RFC3339Nano instant ("2024-03-05T12:00:00.5Z") into signed
 * BigInt nanoseconds since the Unix epoch. Only the "Z" zone, years
 * 0001-9999 and 1-9 fractional digits are accepted; invalid dates throw.
 */
export function parseTimestamp(text) {
  const match = typeof text === "string" ? timestampPattern.exec(text) : null;
  if (!match) {
    throw new TypeError(`not a UTC RFC3339 timestamp: ${JSON.stringify(text)}`);
  }
  const [year, month, day, hour, minute, second] = match.slice(1, 7).map(Number);
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > daysInMonth(year, month) ||
      hour > 23 || minute > 59 || second > 59) {
    throw new TypeError(`timestamp out of range: ${text}`);
  }
  const fraction = BigInt((match[7] ?? "").padEnd(9, "0") || "0");
  const days = BigInt(daysFromCivil(year, month, day));
  const seconds = BigInt(hour * 3600 + minute * 60 + second);
  return days * NANOS_PER_DAY + seconds * NANOS_PER_SECOND + fraction;
}

/**
 * Formats BigInt nanoseconds since the Unix epoch as UTC RFC3339Nano text
 * with trailing fractional zeros trimmed, the inverse of parseTimestamp.
 */
export function formatTimestamp(nanoseconds) {
  let days = nanoseconds / NANOS_PER_DAY;
  let rest = nanoseconds % NANOS_PER_DAY;
  if (rest < 0n) {
    rest += NANOS_PER_DAY;
    days -= 1n;
  }
  const { year, month, day } = civilFromDays(Number(days));
  const totalSeconds = Number(rest / NANOS_PER_SECOND);
  const fraction = rest % NANOS_PER_SECOND;
  const pad = (value, width) => String(value).padStart(width, "0");
  let text = `${pad(year, 4)}-${pad(month, 2)}-${pad(day, 2)}T` +
    `${pad(Math.floor(totalSeconds / 3600), 2)}:${pad(Math.floor(totalSeconds / 60) % 60, 2)}:${pad(totalSeconds % 60, 2)}`;
  if (fraction > 0n) {
    text += "." + pad(fraction, 9).replace(/0+$/, "");
  }
  return text + "Z";
}

/**
 * Formats a signed BigInt nanosecond duration as exact decimal seconds, for
 * example 1500000000n -> "1.5 s" and 0n -> "0 s".
 */
export function formatNanoseconds(nanoseconds) {
  const negative = nanoseconds < 0n;
  const magnitude = negative ? -nanoseconds : nanoseconds;
  const whole = magnitude / NANOS_PER_SECOND;
  const fraction = magnitude % NANOS_PER_SECOND;
  let text = String(whole);
  if (fraction > 0n) {
    text += "." + String(fraction).padStart(9, "0").replace(/0+$/, "");
  }
  return `${negative ? "-" : ""}${text} s`;
}

/**
 * Formats an integer speed in hundredths as an exact multiplier, for
 * example 125 -> "1.25x" and 0 -> "0x", without floating-point drift.
 */
export function formatHundredths(hundredths) {
  if (!Number.isSafeInteger(hundredths) || hundredths < 0) {
    throw new TypeError(`not a nonnegative integer: ${hundredths}`);
  }
  const whole = Math.floor(hundredths / 100);
  const fraction = hundredths % 100;
  if (fraction === 0) {
    return `${whole}x`;
  }
  return `${whole}.${String(fraction).padStart(2, "0").replace(/0$/, "")}x`;
}

/** Converts a millisecond integer into BigInt nanoseconds. */
export function millisecondsToNanoseconds(milliseconds) {
  return BigInt(milliseconds) * 1_000_000n;
}
