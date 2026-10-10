import "@testing-library/jest-dom/vitest";
import { cleanup, configure } from "@testing-library/react";
import { afterAll, afterEach, beforeAll } from "vitest";
import { tokenStore } from "./lib/api/session-store";
import { server } from "./test/server";

// Pages are lazy chunks; the first render in a file pays the dynamic import and transform cost.
configure({ asyncUtilTimeout: 4000 });

// jsdom lacks the pointer/layout APIs Radix Select and Dialog rely on.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= ResizeObserverStub;
Element.prototype.hasPointerCapture ??= () => false;
Element.prototype.setPointerCapture ??= () => {};
Element.prototype.releasePointerCapture ??= () => {};
Element.prototype.scrollIntoView ??= () => {};

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  cleanup();
  server.resetHandlers();
  tokenStore.set(null);
});
afterAll(() => server.close());
