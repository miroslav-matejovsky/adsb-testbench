// Component lifetime tracking.
//
// A component registers every request controller, timer owner, DOM listener
// and cleanup through one lifecycle. destroy() aborts and removes all of
// them exactly once; afterwards `destroyed` is true and callbacks that check
// it do nothing, so a late response can never change a destroyed
// component's DOM or start another request.

export function createLifecycle() {
  let destroyed = false;
  const controllers = new Set();
  const cleanups = [];

  return {
    /** True once destroy() ran. */
    get destroyed() {
      return destroyed;
    },
    /** Number of request controllers still in flight. */
    get pendingRequests() {
      return controllers.size;
    },
    /** Number of registered listeners and cleanups. */
    get registeredCleanups() {
      return cleanups.length;
    },
    /**
     * Returns an AbortController tracked until release() or destroy(). A
     * controller created after destroy is already aborted.
     */
    controller() {
      const controller = new AbortController();
      if (destroyed) {
        controller.abort();
        return { signal: controller.signal, abort: () => {}, release: () => {} };
      }
      controllers.add(controller);
      return {
        signal: controller.signal,
        abort: () => controller.abort(),
        release: () => controllers.delete(controller),
      };
    },
    /** Adds a DOM listener that destroy() removes. */
    listen(target, type, listener, options) {
      if (destroyed) {
        return;
      }
      target.addEventListener(type, listener, options);
      cleanups.push(() => target.removeEventListener(type, listener, options));
    },
    /** Registers a cleanup run by destroy(), in reverse order. */
    onDestroy(cleanup) {
      if (destroyed) {
        cleanup();
        return;
      }
      cleanups.push(cleanup);
    },
    /** Aborts requests and runs cleanups once. Idempotent. */
    destroy() {
      if (destroyed) {
        return;
      }
      destroyed = true;
      for (const controller of controllers) {
        controller.abort();
      }
      controllers.clear();
      while (cleanups.length > 0) {
        const cleanup = cleanups.pop();
        try {
          cleanup();
        } catch (error) {
          console.error("component cleanup failed", error);
        }
      }
    },
  };
}
