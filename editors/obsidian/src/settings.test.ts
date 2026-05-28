// Settings round-trip and change-handler dispatch.
//
// Five controls (plan 214 §Settings): binaryPath, configPath,
// runMode, fixOnSave, traceServer. Each has a typed default.
// Changes to `binaryPath` or `configPath` require a server restart;
// `runMode`, `fixOnSave`, and `traceServer` only need a listener
// reconfigure. The change-classifier surfaces this distinction so
// main.ts can call the right reaction.

import { describe, expect, test } from "bun:test";

import {
  DEFAULTS,
  type MdsmithSettings,
  classifyChange,
  normalize,
} from "./settings";

describe("DEFAULTS", () => {
  test("matches the plan's documented defaults", () => {
    expect(DEFAULTS).toEqual({
      binaryPath: "",
      configPath: "",
      runMode: "onSave",
      fixOnSave: false,
      traceServer: "off",
    });
  });
});

describe("normalize", () => {
  test("returns the defaults when no stored data exists", () => {
    expect(normalize(undefined)).toEqual(DEFAULTS);
    expect(normalize(null)).toEqual(DEFAULTS);
    expect(normalize({})).toEqual(DEFAULTS);
  });

  test("fills in missing fields with their defaults", () => {
    const partial = { binaryPath: "/usr/local/bin/mdsmith" } as Partial<
      MdsmithSettings
    >;
    const out = normalize(partial);
    expect(out.binaryPath).toBe("/usr/local/bin/mdsmith");
    expect(out.runMode).toBe(DEFAULTS.runMode);
    expect(out.fixOnSave).toBe(DEFAULTS.fixOnSave);
  });

  test("ignores unknown keys and bad enum values", () => {
    const dirty = {
      runMode: "neverEver", // not a valid enum value
      traceServer: "blah",
      extraField: 42,
    } as unknown as Partial<MdsmithSettings>;
    const out = normalize(dirty);
    expect(out.runMode).toBe(DEFAULTS.runMode);
    expect(out.traceServer).toBe(DEFAULTS.traceServer);
    // Unknown keys do not leak into the returned object so that
    // saveData round-trips a clean shape.
    expect("extraField" in out).toBe(false);
  });

  test("coerces non-boolean fixOnSave to a boolean", () => {
    // saveData/loadData round-trip through JSON; pin that a "true"
    // string or 1 from a hand-edited file does not slip through.
    expect(normalize({ fixOnSave: "true" } as unknown as Partial<
      MdsmithSettings
    >).fixOnSave).toBe(true);
    expect(normalize({ fixOnSave: 1 } as unknown as Partial<
      MdsmithSettings
    >).fixOnSave).toBe(true);
    expect(normalize({ fixOnSave: 0 } as unknown as Partial<
      MdsmithSettings
    >).fixOnSave).toBe(false);
  });
});

describe("loadData/saveData round-trip integration", () => {
  test("normalize then re-normalize is idempotent (matches JSON round-trip)", () => {
    // Simulate the loadData → normalize → saveData → loadData →
    // normalize sequence. The second normalize on the persisted
    // JSON should hand back the same shape, so an open-close-open
    // cycle does not drift the settings file.
    const initial: Partial<MdsmithSettings> = {
      binaryPath: "/opt/mdsmith",
      runMode: "onType",
      fixOnSave: true,
    };
    const first = normalize(initial);
    const persisted = JSON.parse(JSON.stringify(first));
    const second = normalize(persisted);
    expect(second).toEqual(first);
  });
});

describe("classifyChange", () => {
  test("returns 'restart' when binaryPath changed", () => {
    const before: MdsmithSettings = { ...DEFAULTS };
    const after: MdsmithSettings = { ...DEFAULTS, binaryPath: "/opt/mdsmith" };
    expect(classifyChange(before, after)).toBe("restart");
  });

  test("returns 'restart' when configPath changed", () => {
    const before: MdsmithSettings = { ...DEFAULTS };
    const after: MdsmithSettings = {
      ...DEFAULTS,
      configPath: ".mdsmith.local.yml",
    };
    expect(classifyChange(before, after)).toBe("restart");
  });

  test("returns 'reconfigure' when only runMode/fixOnSave/traceServer changed", () => {
    const before: MdsmithSettings = { ...DEFAULTS };
    const after: MdsmithSettings = {
      ...DEFAULTS,
      runMode: "onType",
      fixOnSave: true,
      traceServer: "verbose",
    };
    expect(classifyChange(before, after)).toBe("reconfigure");
  });

  test("returns 'none' when no fields changed", () => {
    expect(classifyChange({ ...DEFAULTS }, { ...DEFAULTS })).toBe("none");
  });

  test("a binaryPath change wins over a runMode change", () => {
    // Settings panes can flush all five fields at once; the heavier
    // reaction must take precedence so a runMode-only handler does
    // not also run when the server is being restarted.
    const before: MdsmithSettings = { ...DEFAULTS };
    const after: MdsmithSettings = {
      ...DEFAULTS,
      binaryPath: "/opt/mdsmith",
      runMode: "onType",
    };
    expect(classifyChange(before, after)).toBe("restart");
  });
});
