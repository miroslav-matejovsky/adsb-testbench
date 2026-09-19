// Unit tests for manager draft parsing.
import assert from "node:assert/strict";
import { test } from "node:test";

import { parseIntegerDraft } from "../../internal/assets/static/manager.js";

test("integer drafts accept zero and the bound exactly", () => {
  assert.deepEqual(parseIntegerDraft("0", 50, "Count"), { value: 0 });
  assert.deepEqual(parseIntegerDraft(" 50 ", 50, "Count"), { value: 50 });
});

test("integer drafts reject malformed input instead of rounding", () => {
  for (const text of ["", " ", "51", "-1", "+1", "1.0", "1.5", "01", "1e2", "0x10", "abc", "9007199254740993"]) {
    assert.ok(parseIntegerDraft(text, 50, "Count").error, JSON.stringify(text));
  }
});
