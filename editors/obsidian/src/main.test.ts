// Smoke test for the Obsidian plugin entrypoint.
//
// Obsidian loads `dist/main.js` and instantiates the default export
// as a Plugin subclass. The class needs `onload` and `onunload`
// hooks — Obsidian calls them when the user toggles the plugin.
// This test pins the export shape so a future refactor cannot
// accidentally rename or unexport the class.
//
// The `obsidian` module is stubbed by ./test-setup.ts (preload).

import { describe, expect, test } from "bun:test";
import MdsmithPlugin from "./main";

describe("MdsmithPlugin entrypoint", () => {
  test("is the default export", () => {
    // Obsidian's loader looks for the default export from main.js,
    // so the class must be exported that way; a named export would
    // make Obsidian fail to instantiate the plugin.
    expect(MdsmithPlugin).toBeDefined();
    expect(typeof MdsmithPlugin).toBe("function");
  });

  test("instances expose onload and onunload", () => {
    const p = new MdsmithPlugin();
    // Obsidian invokes these on toggle/disable. Until the wiring
    // tasks land they are stubs — the test pins their presence so
    // the bundle keeps loading while we add the LSP wiring.
    expect(typeof p.onload).toBe("function");
    expect(typeof p.onunload).toBe("function");
  });
});
