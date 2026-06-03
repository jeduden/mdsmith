// Settings round-trip and change-handler dispatch.
//
// Plan 217 §Settings declares exactly three controls: configPath,
// runMode, fixOnSave. configPath is the only one that requires a
// runtime restart (plan 215 exposes no in-place reconfigure — a config
// change is dispose + a fresh session); runMode and fixOnSave only need
// a listener reconfigure.

import { describe, expect, test } from "bun:test";

import {
  classifyChange,
  coerceRunMode,
  DEFAULTS,
  type MdsmithSettings,
  normalize,
} from "./settings";

describe("DEFAULTS", () => {
  test("matches the plan's documented three-control defaults", () => {
    expect(DEFAULTS).toEqual({
      configPath: "",
      runMode: "onSave",
      fixOnSave: false,
    });
  });
});

describe("normalize", () => {
  test("returns the defaults when no stored data exists", () => {
    expect(normalize(undefined)).toEqual(DEFAULTS);
    expect(normalize(null)).toEqual(DEFAULTS);
    expect(normalize({})).toEqual(DEFAULTS);
  });

  test("fills missing fields with their defaults", () => {
    expect(normalize({ configPath: "custom.yml" })).toEqual({
      configPath: "custom.yml",
      runMode: "onSave",
      fixOnSave: false,
    });
  });

  test("coerces stringly-typed booleans and rejects junk enums", () => {
    expect(normalize({ fixOnSave: "true" }).fixOnSave).toBe(true);
    expect(normalize({ fixOnSave: "false" }).fixOnSave).toBe(false);
    expect(normalize({ runMode: "garbage" }).runMode).toBe("onSave");
    expect(normalize({ runMode: "onType" }).runMode).toBe("onType");
  });

  test("drops unknown keys so saveData round-trips minimal JSON", () => {
    const out = normalize({
      configPath: "x.yml",
      binaryPath: "/leftover/from/plan-214",
      runMode: "off",
      fixOnSave: true,
    });
    expect(out).toEqual({
      configPath: "x.yml",
      runMode: "off",
      fixOnSave: true,
    });
    expect("binaryPath" in out).toBe(false);
  });
});

describe("classifyChange", () => {
  const base: MdsmithSettings = {
    configPath: "",
    runMode: "onSave",
    fixOnSave: false,
  };

  test("a configPath change requires a restart", () => {
    expect(classifyChange(base, { ...base, configPath: "new.yml" })).toBe(
      "restart",
    );
  });

  test("a runMode or fixOnSave change only needs a reconfigure", () => {
    expect(classifyChange(base, { ...base, runMode: "onType" })).toBe(
      "reconfigure",
    );
    expect(classifyChange(base, { ...base, fixOnSave: true })).toBe(
      "reconfigure",
    );
  });

  test("an identical settings object needs no reaction", () => {
    expect(classifyChange(base, { ...base })).toBe("none");
  });

  test("a configPath change subsumes a simultaneous runMode change", () => {
    expect(
      classifyChange(base, {
        ...base,
        configPath: "new.yml",
        runMode: "off",
      }),
    ).toBe("restart");
  });
});

describe("coerceRunMode", () => {
  test("accepts the allowed run modes and rejects anything else", () => {
    expect(coerceRunMode("onType")).toBe("onType");
    expect(coerceRunMode("onSave")).toBe("onSave");
    expect(coerceRunMode("off")).toBe("off");
    // An unexpected dropdown selection or stored value falls back to the
    // default instead of being persisted verbatim.
    expect(coerceRunMode("garbage")).toBe("onSave");
    expect(coerceRunMode(undefined)).toBe("onSave");
    expect(coerceRunMode(123)).toBe("onSave");
  });
});
