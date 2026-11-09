// jsdom lacks ResizeObserver; Radix primitives (checkbox, radio) measure with it.
if (!('ResizeObserver' in globalThis)) {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
}
