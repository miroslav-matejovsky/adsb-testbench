// Unit tests for bounded JSON requests and the non-overlapping poll loop.
import assert from "node:assert/strict";
import { mock, test } from "node:test";

import { createPoller, ErrorKind, requestJSON } from "../../internal/assets/static/client.js";

const json = { "Content-Type": "application/json; charset=utf-8" };

function respond(status, body, headers = json) {
  return async () => new Response(typeof body === "string" ? body : JSON.stringify(body), { status, headers });
}

const options = (fetchImpl, extra = {}) => ({ timeoutMs: 1000, maxBytes: 4096, fetchImpl, ...extra });

test("successful JSON responses are parsed", async () => {
  const result = await requestJSON("http://h/api/metadata", options(respond(200, { runId: "r" })));
  assert.deepEqual(result, { ok: true, status: 200, body: { runId: "r" }, error: null });
});

test("request bodies are JSON with explicit method and headers", async () => {
  let seen;
  const fetchImpl = async (url, init) => {
    seen = { url, init };
    return new Response("{}", { status: 201, headers: json });
  };
  const result = await requestJSON("http://h/api/stations", options(fetchImpl, { method: "POST", body: { a: 0 } }));
  assert.equal(result.ok, true);
  assert.equal(seen.init.method, "POST");
  assert.equal(seen.init.body, '{"a":0}');
  assert.equal(seen.init.headers["Content-Type"], "application/json");
  assert.equal(seen.init.redirect, "error");
});

test("error envelopes keep code, field and run", async () => {
  const body = { error: { code: "conflict", message: "stale revision", field: "$.expectedRevision", runId: "run-2" } };
  const result = await requestJSON("http://h/x", options(respond(409, body)));
  assert.equal(result.ok, false);
  assert.equal(result.status, 409);
  assert.deepEqual(result.body, body);
  assert.deepEqual(result.error, {
    kind: ErrorKind.http, message: "stale revision", status: 409, code: "conflict",
    field: "$.expectedRevision", runId: "run-2",
  });
});

test("non-2xx display refresh bodies stay available", async () => {
  const body = { snapshot: { status: "stale" }, error: { code: "unavailable", message: "down", field: "", runId: "" } };
  const result = await requestJSON("http://h/observations", options(respond(503, body)));
  assert.equal(result.body.snapshot.status, "stale");
  assert.equal(result.error.code, "unavailable");
  assert.equal(result.error.field, null);
  assert.equal(result.error.runId, null);
});

test("malformed bodies are classified, never thrown", async () => {
  const invalid = await requestJSON("http://h/x", options(respond(200, "{not json")));
  assert.equal(invalid.error.kind, ErrorKind.invalidResponse);
  const wrongType = await requestJSON("http://h/x", options(respond(200, "<html>", { "Content-Type": "text/html" })));
  assert.equal(wrongType.error.kind, ErrorKind.invalidResponse);
  const plain404 = await requestJSON("http://h/x", options(respond(404, "missing", { "Content-Type": "text/plain" })));
  assert.equal(plain404.error.kind, ErrorKind.http);
  assert.equal(plain404.error.status, 404);
  const noEnvelope = await requestJSON("http://h/x", options(respond(500, { other: true })));
  assert.equal(noEnvelope.error.kind, ErrorKind.http);
  assert.equal(noEnvelope.error.code, null);
  const invalidUtf8 = async () => new Response(new Uint8Array([0x22, 0xff, 0x22]), { status: 200, headers: json });
  assert.equal((await requestJSON("http://h/x", options(invalidUtf8))).error.kind, ErrorKind.invalidResponse);
});

test("oversized responses are rejected whatever their declared length", async () => {
  const large = JSON.stringify({ data: "x".repeat(5000) });
  const result = await requestJSON("http://h/x", options(respond(200, large)));
  assert.equal(result.error.kind, ErrorKind.responseLimit);
  const streamed = async () => new Response(new ReadableStream({
    start(controller) {
      for (let i = 0; i < 10; i++) {
        controller.enqueue(new TextEncoder().encode("[" + "1,".repeat(300)));
      }
      controller.close();
    },
  }), { status: 200, headers: json });
  assert.equal((await requestJSON("http://h/x", options(streamed))).error.kind, ErrorKind.responseLimit);
});

test("network failure, cancellation and deadline are distinct", async () => {
  const network = await requestJSON("http://h/x", options(async () => {
    throw new TypeError("connection refused");
  }));
  assert.equal(network.error.kind, ErrorKind.network);

  const hang = (url, init) => new Promise((_, reject) => {
    init.signal.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
  });
  const controller = new AbortController();
  const pending = requestJSON("http://h/x", options(hang, { signal: controller.signal }));
  controller.abort();
  assert.equal((await pending).error.kind, ErrorKind.aborted);

  const already = new AbortController();
  already.abort();
  let called = false;
  const skipped = await requestJSON("http://h/x", options(async () => {
    called = true;
  }, { signal: already.signal }));
  assert.equal(skipped.error.kind, ErrorKind.aborted);
  assert.equal(called, false, "an aborted request is never sent");

  mock.timers.enable({ apis: ["setTimeout"] });
  try {
    const timed = requestJSON("http://h/x", options(hang, { timeoutMs: 50 }));
    mock.timers.tick(50);
    assert.equal((await timed).error.kind, ErrorKind.timeout);
  } finally {
    mock.timers.reset();
  }
});

test("the poller runs one cycle at a time and schedules after completion", async () => {
  mock.timers.enable({ apis: ["setTimeout"] });
  try {
    let running = 0;
    let started = 0;
    const releases = [];
    const poller = createPoller({
      intervalMs: 100,
      run: () => new Promise((resolve) => {
        running += 1;
        started += 1;
        assert.equal(running, 1, "cycles never overlap");
        releases.push(() => {
          running -= 1;
          resolve();
        });
      }),
    });
    poller.start();
    assert.equal(started, 1);
    mock.timers.tick(1000);
    assert.equal(started, 1, "no new cycle while one is in flight");
    releases.shift()();
    await Promise.resolve();
    await Promise.resolve();
    mock.timers.tick(99);
    assert.equal(started, 1);
    mock.timers.tick(1);
    assert.equal(started, 2, "the next cycle starts one interval after completion");

    poller.refreshNow();
    assert.equal(started, 2, "refreshNow waits for the running cycle");
    releases.shift()();
    await Promise.resolve();
    await Promise.resolve();
    assert.equal(started, 3, "the deferred refresh runs immediately after");

    poller.stop();
    releases.shift()();
    await Promise.resolve();
    await Promise.resolve();
    mock.timers.tick(10000);
    assert.equal(started, 3, "a stopped poller schedules nothing");
  } finally {
    mock.timers.reset();
  }
});

test("stop aborts the in-flight cycle signal", async () => {
  let signal;
  const poller = createPoller({
    intervalMs: 100,
    run: (cycleSignal) => {
      signal = cycleSignal;
      return new Promise(() => {});
    },
  });
  poller.start();
  assert.equal(signal.aborted, false);
  poller.stop();
  poller.stop();
  assert.equal(signal.aborted, true);
});

test("restart aborts the running cycle and never leaves two loops", async () => {
  mock.timers.enable({ apis: ["setTimeout"] });
  try {
    let started = 0;
    const signals = [];
    const releases = [];
    const poller = createPoller({
      intervalMs: 100,
      run: (signal) => new Promise((resolve) => {
        started += 1;
        signals.push(signal);
        releases.push(resolve);
      }),
    });
    poller.start();
    poller.restart();
    assert.equal(signals[0].aborted, true, "the superseded cycle is aborted");
    assert.equal(started, 2);
    releases[0]();
    releases[1]();
    for (let i = 0; i < 4; i++) {
      await Promise.resolve();
    }
    mock.timers.tick(100);
    assert.equal(started, 3, "exactly one loop continues");
    poller.stop();
  } finally {
    mock.timers.reset();
  }
});
