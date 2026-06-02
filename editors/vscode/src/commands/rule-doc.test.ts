import { describe, expect, test } from "bun:test";
import type { SpawnFn } from "./runner";
import {
  buildRuleDocUri,
  fetchRuleDocContent,
  isRuleId,
  OPEN_RULE_DOC_COMMAND,
  parseRuleDocUri,
  rewriteHoverMarkdown,
  rewriteRuleDocLinks,
  ruleDocCommandUri,
  RULE_SCHEME,
  type MarkdownLike,
} from "./rule-doc";

// ---- isRuleId ----

describe("isRuleId", () => {
  test("accepts MDS followed by digits, case-insensitive", () => {
    expect(isRuleId("MDS020")).toBe(true);
    expect(isRuleId("mds1")).toBe(true);
  });

  test("rejects slugs, partials, flags, and empties", () => {
    for (const bad of ["", "MDS", "MDS01a", "required-structure", "mds020-required-structure", "--version"]) {
      expect(isRuleId(bad)).toBe(false);
    }
  });
});

// ---- buildRuleDocUri / parseRuleDocUri ----

describe("buildRuleDocUri", () => {
  test("encodes the rule ID in the query, preserving case", () => {
    expect(buildRuleDocUri("MDS020")).toBe(`${RULE_SCHEME}://doc?id=MDS020`);
  });

  test("round-trips through parseRuleDocUri", () => {
    expect(parseRuleDocUri(buildRuleDocUri("MDS001"))).toEqual({ id: "MDS001" });
  });
});

describe("parseRuleDocUri", () => {
  test("extracts a well-formed rule ID", () => {
    expect(parseRuleDocUri("mdsmith-rule://doc?id=MDS042")).toEqual({ id: "MDS042" });
  });

  test("rejects the wrong scheme", () => {
    expect(parseRuleDocUri("mdsmith-kinds://doc?id=MDS001")).toBeNull();
  });

  test("rejects the wrong host", () => {
    expect(parseRuleDocUri("mdsmith-rule://rule?id=MDS001")).toBeNull();
  });

  test("rejects a missing id", () => {
    expect(parseRuleDocUri("mdsmith-rule://doc")).toBeNull();
  });

  test("rejects a malformed URI", () => {
    expect(parseRuleDocUri("not a uri")).toBeNull();
  });

  test("rejects an id that is not a rule ID (defense-in-depth)", () => {
    // A crafted URI must never smuggle an arbitrary argument into the
    // spawned `mdsmith help rule`.
    expect(parseRuleDocUri("mdsmith-rule://doc?id=--version")).toBeNull();
    expect(parseRuleDocUri("mdsmith-rule://doc?id=required-structure")).toBeNull();
  });
});

// ---- ruleDocCommandUri ----

describe("ruleDocCommandUri", () => {
  test("builds a command URI with the ID as a JSON argument", () => {
    expect(ruleDocCommandUri("MDS020")).toBe(
      `command:${OPEN_RULE_DOC_COMMAND}?${encodeURIComponent('["MDS020"]')}`,
    );
  });
});

// ---- rewriteRuleDocLinks ----

describe("rewriteRuleDocLinks", () => {
  test("rewrites a published rule-docs link to the offline command", () => {
    const md = "[Open rule docs ↗](https://mdsmith.dev/rules/mds020-required-structure/)";
    expect(rewriteRuleDocLinks(md)).toBe(
      `[Open rule docs ↗](${ruleDocCommandUri("MDS020")})`,
    );
  });

  test("preserves the link label", () => {
    const md = "see [the docs](https://mdsmith.dev/rules/mds001-line-length/) now";
    expect(rewriteRuleDocLinks(md)).toBe(
      `see [the docs](${ruleDocCommandUri("MDS001")}) now`,
    );
  });

  test("is host-agnostic", () => {
    const md = "[d](https://docs.example.test/rules/mds007-no-tabs)";
    expect(rewriteRuleDocLinks(md)).toBe(`[d](${ruleDocCommandUri("MDS007")})`);
  });

  test("rewrites every link when several are present", () => {
    const md =
      "[a](https://mdsmith.dev/rules/mds001-x/) and [b](https://mdsmith.dev/rules/mds002-y/)";
    expect(rewriteRuleDocLinks(md)).toBe(
      `[a](${ruleDocCommandUri("MDS001")}) and [b](${ruleDocCommandUri("MDS002")})`,
    );
  });

  test("leaves markdown without a rule-docs link untouched", () => {
    const md = "MDS020 — see [home](https://mdsmith.dev/) for more";
    expect(rewriteRuleDocLinks(md)).toBe(md);
  });
});

// ---- rewriteHoverMarkdown ----

describe("rewriteHoverMarkdown", () => {
  test("rewrites and trusts only the open-rule-doc command", () => {
    const md: MarkdownLike = {
      value: "[Open rule docs ↗](https://mdsmith.dev/rules/mds020-required-structure/)",
    };
    expect(rewriteHoverMarkdown(md)).toBe(true);
    expect(md.value).toBe(`[Open rule docs ↗](${ruleDocCommandUri("MDS020")})`);
    expect(md.isTrusted).toEqual({ enabledCommands: [OPEN_RULE_DOC_COMMAND] });
  });

  test("leaves an unrelated block untouched, including its trust level", () => {
    const md: MarkdownLike = { value: "just a message", isTrusted: false };
    expect(rewriteHoverMarkdown(md)).toBe(false);
    expect(md.value).toBe("just a message");
    expect(md.isTrusted).toBe(false);
  });
});

// ---- fetchRuleDocContent ----

const fakeSpawn =
  (result: { stdout?: string; stderr?: string; exitCode?: number }, capture?: (args: string[]) => void): SpawnFn =>
  async (_binary, args) => {
    capture?.(args);
    return { stdout: result.stdout ?? "", stderr: result.stderr ?? "", exitCode: result.exitCode ?? 0 };
  };

describe("fetchRuleDocContent", () => {
  test("returns the embedded README on success and passes the bare ID", async () => {
    let seen: string[] = [];
    const out = await fetchRuleDocContent(
      buildRuleDocUri("MDS020"),
      "mdsmith",
      undefined,
      fakeSpawn({ stdout: "# MDS020: required-structure\n\nbody\n" }, (a) => (seen = a)),
    );
    expect(seen).toEqual(["help", "rule", "MDS020"]);
    expect(out).toBe("# MDS020: required-structure\n\nbody\n");
  });

  test("reports a malformed URI without spawning", async () => {
    let spawned = false;
    const out = await fetchRuleDocContent(
      "mdsmith-rule://doc?id=nope",
      "mdsmith",
      undefined,
      fakeSpawn({}, () => (spawned = true)),
    );
    expect(spawned).toBe(false);
    expect(out).toContain("malformed rule URI");
  });

  test("reports a non-zero exit with stderr", async () => {
    const out = await fetchRuleDocContent(
      buildRuleDocUri("MDS999"),
      "mdsmith",
      undefined,
      fakeSpawn({ stderr: "mdsmith: unknown rule MDS999", exitCode: 2 }),
    );
    expect(out).toContain("failed (exit 2)");
    expect(out).toContain("unknown rule MDS999");
  });

  test("reports a spawn failure", async () => {
    const failing: SpawnFn = async () => {
      throw new Error("ENOENT");
    };
    const out = await fetchRuleDocContent(buildRuleDocUri("MDS001"), "mdsmith", undefined, failing);
    expect(out).toContain("could not start");
    expect(out).toContain("ENOENT");
  });
});
