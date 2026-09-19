// Test-only harness: exposes the public component API so browser tests can
// mount, destroy and remount components into two independent roots.
import * as components from "/assets/components.js";

window.tbHarness = {
  ...components,
  handles: {},
  mount(kind, rootId, config) {
    const root = document.getElementById(rootId);
    const mount = kind === "manager" ? components.mountManager : components.mountAircraftDisplay;
    const handle = mount(root, config);
    this.handles[rootId] = handle;
    return true;
  },
  destroy(rootId) {
    this.handles[rootId].destroy();
  },
};
window.tbHarnessReady = true;
