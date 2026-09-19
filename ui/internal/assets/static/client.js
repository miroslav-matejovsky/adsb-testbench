// Bounded JSON requests and a non-overlapping poll loop.
//
// requestJSON never throws for transport or protocol problems. It resolves to
// a result object so every caller handles failure explicitly:
//
//   { ok, status, body, error }
//
// ok is true only for a 2xx response with a valid JSON body. body is the
// parsed JSON whenever one was received, including on non-2xx statuses, so a
// display RefreshResponse with stale data remains usable. error is null on
// success and otherwise { kind, message, status, code, field, runId } where
// kind is one of: "aborted", "timeout", "network", "response-limit",
// "invalid-response", "http". Requests are never retried.

const jsonMediaType = /^application\/json\s*(;\s*charset\s*=\s*"?utf-8"?\s*)?$/i;

/** Error kinds reported in result.error.kind. */
export const ErrorKind = Object.freeze({
  aborted: "aborted",
  timeout: "timeout",
  network: "network",
  responseLimit: "response-limit",
  invalidResponse: "invalid-response",
  http: "http",
});

function failure(kind, message, extra = {}) {
  return {
    kind, message,
    status: extra.status ?? null,
    code: extra.code ?? null,
    field: extra.field ?? null,
    runId: extra.runId ?? null,
  };
}

// readBounded reads at most maxBytes from a response body and reports
// overflow without buffering the excess.
async function readBounded(response, maxBytes) {
  const declared = Number(response.headers.get("Content-Length"));
  if (Number.isFinite(declared) && declared > maxBytes) {
    await response.body?.cancel();
    return { overflow: true };
  }
  if (!response.body) {
    return { bytes: new Uint8Array(0) };
  }
  const reader = response.body.getReader();
  const chunks = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    total += value.byteLength;
    if (total > maxBytes) {
      await reader.cancel();
      return { overflow: true };
    }
    chunks.push(value);
  }
  const bytes = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return { bytes };
}

// envelopeError extracts the shared error envelope carried by a failed
// simulator or display response.
function envelopeError(status, body) {
  const envelope = body && typeof body === "object" ? body.error : null;
  if (envelope && typeof envelope === "object" && typeof envelope.code === "string" &&
      typeof envelope.message === "string") {
    return failure(ErrorKind.http, envelope.message, {
      status,
      code: envelope.code,
      field: typeof envelope.field === "string" && envelope.field !== "" ? envelope.field : null,
      runId: typeof envelope.runId === "string" && envelope.runId !== "" ? envelope.runId : null,
    });
  }
  return failure(ErrorKind.http, `the server responded with status ${status}`, { status });
}

/**
 * Sends one bounded JSON request.
 *
 * options: { method, body (serialized with JSON.stringify when present),
 * signal (caller cancellation), timeoutMs, maxBytes, fetchImpl }.
 */
export async function requestJSON(url, options) {
  const { method = "GET", body, signal, timeoutMs, maxBytes } = options;
  const fetchImpl = options.fetchImpl ?? globalThis.fetch.bind(globalThis);
  if (signal?.aborted) {
    return { ok: false, status: null, body: null, error: failure(ErrorKind.aborted, "the request was canceled") };
  }
  const controller = new AbortController();
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, timeoutMs);
  const forwardAbort = () => controller.abort();
  signal?.addEventListener("abort", forwardAbort, { once: true });

  const aborted = () => timedOut
    ? failure(ErrorKind.timeout, `the request exceeded ${timeoutMs} ms`)
    : failure(ErrorKind.aborted, "the request was canceled");

  try {
    const init = { method, signal: controller.signal, headers: { Accept: "application/json" }, cache: "no-store", redirect: "error" };
    if (body !== undefined) {
      init.body = JSON.stringify(body);
      init.headers["Content-Type"] = "application/json";
    }
    let response;
    try {
      response = await fetchImpl(url, init);
    } catch (error) {
      if (controller.signal.aborted) {
        return { ok: false, status: null, body: null, error: aborted() };
      }
      return { ok: false, status: null, body: null, error: failure(ErrorKind.network, `the request failed: ${error?.message ?? error}`) };
    }

    const status = response.status;
    const contentType = response.headers.get("Content-Type") ?? "";
    if (!jsonMediaType.test(contentType.trim())) {
      await response.body?.cancel();
      const error = response.ok
        ? failure(ErrorKind.invalidResponse, `unexpected content type ${JSON.stringify(contentType)}`, { status })
        : failure(ErrorKind.http, `the server responded with status ${status}`, { status });
      return { ok: false, status, body: null, error };
    }

    let read;
    try {
      read = await readBounded(response, maxBytes);
    } catch (error) {
      if (controller.signal.aborted) {
        return { ok: false, status, body: null, error: aborted() };
      }
      return { ok: false, status, body: null, error: failure(ErrorKind.network, `reading the response failed: ${error?.message ?? error}`, { status }) };
    }
    if (read.overflow) {
      return { ok: false, status, body: null, error: failure(ErrorKind.responseLimit, `the response exceeds ${maxBytes} bytes`, { status }) };
    }

    let parsed;
    try {
      parsed = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(read.bytes));
    } catch {
      return { ok: false, status, body: null, error: failure(ErrorKind.invalidResponse, "the response is not valid JSON", { status }) };
    }
    if (status < 200 || status > 299) {
      return { ok: false, status, body: parsed, error: envelopeError(status, parsed) };
    }
    return { ok: true, status, body: parsed, error: null };
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener("abort", forwardAbort);
  }
}

/**
 * Creates a poll loop that runs one cycle at a time. The next cycle is
 * scheduled intervalMs after the previous one completed, so cycles never
 * overlap. run(signal) must return a promise; stop() aborts its signal.
 */
export function createPoller({ intervalMs, run }) {
  let timer = null;
  let controller = null;
  let running = false;
  let rerun = false;
  let stopped = true;
  // epoch changes on every stop, so a cycle superseded by stop/restart can
  // never schedule another cycle or clear the state of its successor.
  let epoch = 0;

  const schedule = () => {
    if (!stopped) {
      timer = setTimeout(cycle, intervalMs);
    }
  };

  async function cycle() {
    timer = null;
    if (stopped) {
      return;
    }
    const own = epoch;
    running = true;
    controller = new AbortController();
    try {
      await run(controller.signal);
    } catch (error) {
      // A cycle failure must not stop polling; the cycle reports its own
      // errors, so this only guards against programming faults.
      console.error("poll cycle failed", error);
    }
    if (own !== epoch) {
      return;
    }
    running = false;
    controller = null;
    if (rerun) {
      rerun = false;
      cycle();
      return;
    }
    schedule();
  }

  return {
    /** Starts polling with an immediate cycle. */
    start() {
      if (!stopped) {
        return;
      }
      stopped = false;
      cycle();
    },
    /** Runs a cycle as soon as the current one, if any, completes. */
    refreshNow() {
      if (stopped) {
        return;
      }
      if (running) {
        rerun = true;
        return;
      }
      clearTimeout(timer);
      cycle();
    },
    /**
     * Aborts the in-flight cycle, if any, and starts a new cycle at once.
     * Used when a newer request supersedes the running one.
     */
    restart() {
      if (stopped) {
        return;
      }
      this.stop();
      this.start();
    },
    /** Stops polling and aborts the in-flight cycle. Idempotent. */
    stop() {
      stopped = true;
      epoch += 1;
      running = false;
      rerun = false;
      clearTimeout(timer);
      timer = null;
      controller?.abort();
    },
  };
}
