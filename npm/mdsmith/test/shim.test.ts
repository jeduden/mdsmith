import { describe, expect, test } from "bun:test";

import {
  PLATFORM_PACKAGES,
  platformPackage,
  binaryRelativePath,
  resolveBinary,
} from "../bin/mdsmith.js";

describe("platformPackage", () => {
  test.each([
    ["linux", "x64", "@mdsmith/linux-x64"],
    ["linux", "arm64", "@mdsmith/linux-arm64"],
    ["darwin", "x64", "@mdsmith/darwin-x64"],
    ["darwin", "arm64", "@mdsmith/darwin-arm64"],
    ["win32", "x64", "@mdsmith/win32-x64"],
  ])(
    "%s/%s resolves to %s",
    (platform: string, arch: string, expected: string) => {
      expect(platformPackage(platform, arch)).toBe(expected);
    },
  );

  test("unsupported platform returns undefined", () => {
    expect(platformPackage("freebsd", "x64")).toBeUndefined();
    expect(platformPackage("linux", "ia32")).toBeUndefined();
  });

  test("PLATFORM_PACKAGES is frozen so callers cannot drift the map", () => {
    expect(() => {
      // @ts-expect-error — the test asserts immutability at runtime.
      PLATFORM_PACKAGES["linux-x64"] = "@evil/package";
    }).toThrow();
  });
});

describe("binaryRelativePath", () => {
  test("appends .exe on win32", () => {
    expect(binaryRelativePath("win32")).toBe("bin/mdsmith.exe");
  });

  test("plain mdsmith on POSIX platforms", () => {
    expect(binaryRelativePath("linux")).toBe("bin/mdsmith");
    expect(binaryRelativePath("darwin")).toBe("bin/mdsmith");
  });
});

describe("resolveBinary", () => {
  test("resolves through the supplied resolver to the platform binary", () => {
    const calls: string[] = [];
    const fakeResolve = (id: string) => {
      calls.push(id);
      return `/fake/node_modules/${id}`;
    };

    const bin = resolveBinary("linux", "arm64", fakeResolve);

    expect(calls).toEqual(["@mdsmith/linux-arm64/bin/mdsmith"]);
    expect(bin).toBe(
      "/fake/node_modules/@mdsmith/linux-arm64/bin/mdsmith",
    );
  });

  test("uses the .exe suffix on win32", () => {
    const calls: string[] = [];
    const fakeResolve = (id: string) => {
      calls.push(id);
      return `/fake/${id}`;
    };

    resolveBinary("win32", "x64", fakeResolve);

    expect(calls).toEqual(["@mdsmith/win32-x64/bin/mdsmith.exe"]);
  });

  test("unsupported platform throws with a discoverable code", () => {
    const fakeResolve = () => {
      throw new Error("resolve should not be called");
    };
    let caught: unknown;
    try {
      resolveBinary("freebsd", "x64", fakeResolve);
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(Error);
    expect((caught as { code?: string }).code).toBe(
      "MDSMITH_UNSUPPORTED_PLATFORM",
    );
    expect((caught as Error).message).toContain("freebsd-x64");
  });

  test("missing platform package surfaces a clear reinstall hint", () => {
    const fakeResolve = (_id: string) => {
      const err = new Error("Cannot find module");
      throw err;
    };
    let caught: unknown;
    try {
      resolveBinary("linux", "x64", fakeResolve);
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(Error);
    expect((caught as { code?: string }).code).toBe(
      "MDSMITH_PLATFORM_PACKAGE_MISSING",
    );
    expect((caught as Error).message).toContain(
      "@mdsmith/linux-x64",
    );
    expect((caught as Error).message).toContain(
      "github.com/jeduden/mdsmith/releases",
    );
  });
});
