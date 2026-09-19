// Shared component scaffolding: root ownership, scoped wrapper, status and
// error regions, and stylesheet loading from the asset base.

import { h } from "./dom.js";
import { createLifecycle } from "./lifecycle.js";

const mountedAttribute = "data-tb-mounted";

/**
 * Ensures a stylesheet from the asset base is present in the document. A
 * stylesheet already linked by a page or another component is reused.
 */
export function ensureStylesheet(href) {
  for (const link of document.querySelectorAll('link[rel="stylesheet"]')) {
    if (link.href === href) {
      return;
    }
  }
  document.head.append(h("link", { rel: "stylesheet", href }));
}

/**
 * Claims root for one component and builds its scoped wrapper.
 *
 * Returns { lifecycle, wrapper, status, error, content }. status is a polite
 * live region for progress; error is an alert region for actionable
 * failures; content holds the component body. destroy() of the lifecycle
 * removes the wrapper and releases root, leaving other roots untouched.
 */
export function createScaffold(root, { kind, label }) {
  if (!(root instanceof Element)) {
    throw new TypeError("root must be a DOM element");
  }
  if (root.hasAttribute(mountedAttribute)) {
    throw new Error("root already hosts a mounted component; destroy it first");
  }
  const lifecycle = createLifecycle();
  const status = h("p", { class: "tb-status", role: "status", "aria-live": "polite" }, "Loading...");
  const error = h("div", { class: "tb-error", role: "alert", hidden: true });
  const content = h("div", { class: "tb-content" });
  const wrapper = h("section", { class: `tb-component tb-${kind}`, "aria-label": label },
    status, error, content);

  root.setAttribute(mountedAttribute, kind);
  root.replaceChildren(wrapper);
  lifecycle.onDestroy(() => {
    wrapper.remove();
    root.removeAttribute(mountedAttribute);
  });
  return { lifecycle, wrapper, status, error, content };
}

/**
 * Wraps a poll cycle so the component wrapper carries aria-busy="true"
 * while the cycle runs. Assistive technology can defer announcing a
 * half-updated view, and hosts can tell when a refresh has settled. A
 * destroyed component's wrapper is never touched.
 */
export function busyWhile(wrapper, lifecycle, run) {
  return async (signal) => {
    if (!lifecycle.destroyed) {
      wrapper.setAttribute("aria-busy", "true");
    }
    try {
      await run(signal);
    } finally {
      if (!lifecycle.destroyed) {
        wrapper.removeAttribute("aria-busy");
      }
    }
  };
}

/**
 * Shows an actionable failure in an error region, or hides it when failure
 * is null. failure is a client result error or { message }.
 */
export function showError(region, failure, prefix) {
  if (!failure) {
    region.hidden = true;
    region.replaceChildren();
    return;
  }
  const parts = [prefix ? `${prefix}: ` : "", failure.message];
  if (failure.code) {
    parts.push(` (code ${failure.code}`, failure.field ? `, field ${failure.field}` : "", ")");
  }
  region.replaceChildren(document.createTextNode(parts.join("")));
  region.hidden = false;
}
