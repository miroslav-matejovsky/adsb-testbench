// Public browser entry point of the ADS-B test bench components.
//
//   import { mountManager, mountAircraftDisplay } from "<assetBase>components.js";
//   const handle = mountManager(rootElement, config);
//   handle.destroy();
//
// Each mount validates its complete configuration and resolves its URL
// bases against the page origin before touching the DOM or sending any
// request; an invalid configuration throws ConfigError. A mount owns only
// its root element, requests, timers, listeners and map. destroy() is
// idempotent, aborts in-flight requests, stops polling, removes listeners
// and the map, and releases the root for a new mount.

export { mountManager } from "./manager.js";
export { mountAircraftDisplay } from "./aircraft.js";
export { ConfigError } from "./config.js";
