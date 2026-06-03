import { describe, expect, test } from "bun:test";
import { URI } from "vscode-uri";
import {
  extractRuleIds,
  makeKindsContentProvider,
  runKindsResolve,
  runKindsWhy,
  type DiagnosticLike,
  type KindsWhyHandlerDeps
} from "./kinds";
import {
  buildResolveUri,
  buildWhyUri,
  kindsContentUri,
  parseKindsUri,
  fetchKindsContent,
} from "./virtual-doc";

// ---- extractRuleIds ----

describe("extractRuleIds", () => {
  test("returns empty array when no diagnostics", () => {
    expect(extractRuleIds([])).toEqual([]);
  });

  test("extracts string codes from mdsmith diagnostics", () => {
    const diags: DiagnosticLike[] = [
      { source: "mdsmith", code: "MDS001" },
      { source: "mdsmith", code: "MDS005" },
    ];
    expect(extractRuleIds(diags)).toEqual(["MDS001", "MDS005"]);
  });

  test("ignores diagnostics from other sources", () => {
    const diags: DiagnosticLike[] = [
      { source: "eslint", code: "no-unused-vars" },
      { source: "mdsmith", code: "MDS003" },
    ];
    expect(extractRuleIds(diags)).toEqual(["MDS003"]);
  });

  test("deduplicates rule IDs", () => {
    const diags: DiagnosticLike[] = [
      { source: "mdsmith", code: "MDS001" },
      { source: "mdsmith", code: "MDS001" },
    ];
    expect(extractRuleIds(diags)).toEqual(["MDS001"]);
  });

  test("returns sorted IDs", () => {
    const diags: DiagnosticLike[] = [
      { source: "mdsmith", code: "MDS010" },
      { source: "mdsmith", code: "MDS002" },
    ];
    expect(extractRuleIds(diags)).toEqual(["MDS002", "MDS010"]);
  });

  test("ignores numeric codes", () => {
    const diags: DiagnosticLike[] = [
      { source: "mdsmith", code: 42 },
    ];
    expect(extractRuleIds(diags)).toEqual([]);
  });
});

// ---- buildResolveUri / buildWhyUri / parseKindsUri ----

describe("buildResolveUri", () => {
  test("produces a valid mdsmith-kinds:// URI", () => {
    const uri = buildResolveUri("/workspace/docs/guide.md");
    expect(uri).toBe(
      "mdsmith-kinds://resolve?file=%2Fworkspace%2Fdocs%2Fguide.md"
    );
  });
});

describe("buildWhyUri", () => {
  test("produces a valid mdsmith-kinds:// URI with rule", () => {
    const uri = buildWhyUri("/workspace/docs/guide.md", "MDS001");
    expect(uri).toBe(
      "mdsmith-kinds://why?file=%2Fworkspace%2Fdocs%2Fguide.md&rule=MDS001"
    );
  });
});

describe("parseKindsUri", () => {
  test("parses a resolve URI", () => {
    const uri = buildResolveUri("/repo/foo.md");
    expect(parseKindsUri(uri)).toEqual({
      command: "resolve",
      file: "/repo/foo.md",
      rule: undefined,
    });
  });

  test("parses a why URI", () => {
    const uri = buildWhyUri("/repo/foo.md", "MDS001");
    expect(parseKindsUri(uri)).toEqual({
      command: "why",
      file: "/repo/foo.md",
      rule: "MDS001",
    });
  });

  test("returns null for malformed URI", () => {
    expect(parseKindsUri("mdsmith-kinds://unknown?file=/foo.md")).toBeNull();
    expect(parseKindsUri("http://example.com")).toBeNull();
    expect(parseKindsUri("mdsmith-kinds://why?rule=MDS001")).toBeNull();
    expect(parseKindsUri("mdsmith-kinds://why?file=/foo.md")).toBeNull();
  });
});

// ---- kindsContentUri (vscode.Uri round trip) ----

describe("kindsContentUri", () => {
  // The content provider receives the Uri VS Code parsed from
  // buildResolveUri/buildWhyUri and must turn it back into a string
  // parseKindsUri can read. The default Uri.toString() percent-encodes
  // the query separators ('=' -> %3D, and the param-joining '&' -> %26),
  // so the file/rule keys vanish and the provider reports "malformed
  // kinds URI". URI here is the same implementation backing vscode.Uri.
  test("round-trips a resolve URI through parseKindsUri", () => {
    const uri = URI.parse(buildResolveUri("/a/b.md"));
    expect(uri.toString()).toBe("mdsmith-kinds://resolve?file%3D%2Fa%2Fb.md");
    expect(parseKindsUri(kindsContentUri(uri))).toEqual({
      command: "resolve",
      file: "/a/b.md",
      rule: undefined,
    });
  });

  test("round-trips a why URI (two query params) through parseKindsUri", () => {
    const uri = URI.parse(buildWhyUri("/a/b.md", "MDS001"));
    expect(parseKindsUri(kindsContentUri(uri))).toEqual({
      command: "why",
      file: "/a/b.md",
      rule: "MDS001",
    });
  });
});

// ---- fetchKindsContent ----

describe("fetchKindsContent", () => {
  test("returns fenced JSON for a successful resolve", async () => {
    const spawn = async (_bin: string, _args: string[]) => ({
      stdout: '{"kinds":["plan"]}',
      stderr: "",
      exitCode: 0,
    });
    const uri = buildResolveUri("/repo/foo.md");
    const content = await fetchKindsContent(uri, "mdsmith", "/repo", spawn);
    expect(content).toContain("~~~json");
    expect(content).toContain('{"kinds":["plan"]}');
    expect(content).toContain("Resolved config: /repo/foo.md");
  });

  test("returns fenced JSON for a successful why", async () => {
    const spawn = async (_bin: string, _args: string[]) => ({
      stdout: '{"rule":"MDS001","layers":[]}',
      stderr: "",
      exitCode: 0,
    });
    const uri = buildWhyUri("/repo/foo.md", "MDS001");
    const content = await fetchKindsContent(uri, "mdsmith", "/repo", spawn);
    expect(content).toContain("Rule MDS001 on /repo/foo.md");
    expect(content).toContain("~~~json");
  });

  test("returns error block on non-zero exit", async () => {
    const spawn = async () => ({
      stdout: "",
      stderr: "kinds resolve: config not found\n",
      exitCode: 2,
    });
    const uri = buildResolveUri("/repo/foo.md");
    const content = await fetchKindsContent(uri, "mdsmith", "/repo", spawn);
    expect(content).toContain("failed");
    expect(content).toContain("config not found");
  });

  test("returns error for malformed URI", async () => {
    const spawn = async () => ({ stdout: "", stderr: "", exitCode: 0 });
    const content = await fetchKindsContent("invalid://uri", "mdsmith", undefined, spawn);
    expect(content).toContain("malformed");
  });

  test("returns error block when spawn rejects (e.g. binary not found)", async () => {
    const spawn = async () => { throw new Error("ENOENT: mdsmith"); };
    const uri = buildResolveUri("/repo/foo.md");
    const content = await fetchKindsContent(uri, "mdsmith", undefined, spawn);
    expect(content).toContain("could not start");
    expect(content).toContain("ENOENT");
  });

  test("passes correct args for resolve subcommand", async () => {
    let capturedArgs: string[] = [];
    const spawn = async (_bin: string, args: string[]) => {
      capturedArgs = args;
      return { stdout: "{}", stderr: "", exitCode: 0 };
    };
    await fetchKindsContent(buildResolveUri("/repo/a.md"), "mdsmith", undefined, spawn);
    expect(capturedArgs).toEqual(["kinds", "resolve", "--json", "--", "/repo/a.md"]);
  });

  test("passes correct args for why subcommand", async () => {
    let capturedArgs: string[] = [];
    const spawn = async (_bin: string, args: string[]) => {
      capturedArgs = args;
      return { stdout: "{}", stderr: "", exitCode: 0 };
    };
    await fetchKindsContent(buildWhyUri("/repo/a.md", "MDS005"), "mdsmith", undefined, spawn);
    expect(capturedArgs).toEqual(["kinds", "why", "--json", "--", "/repo/a.md", "MDS005"]);
  });
});

// ---- runKindsResolve / runKindsWhy ----

function makeDeps(overrides: Partial<KindsWhyHandlerDeps> = {}): {
  deps: KindsWhyHandlerDeps;
  openedUris: string[];
  errors: string[];
} {
  const openedUris: string[] = [];
  const errors: string[] = [];
  const deps: KindsWhyHandlerDeps = {
    getActiveFilePath: () => "/repo/doc.md",
    getDiagnostics: () => [],
    pickRule: async (rules) => rules[0],
    openVirtualDoc: async (uri) => { openedUris.push(uri); },
    showError: async (msg) => { errors.push(msg); },
    ...overrides,
  };
  return { deps, openedUris, errors };
}

describe("makeKindsContentProvider", () => {
  test("provideTextDocumentContent delegates to fetchKindsContent via spawn", async () => {
    const spawn = async (_bin: string, _args: string[]) => ({
      stdout: '{"file":"a.md"}',
      stderr: "",
      exitCode: 0,
    });
    const provider = makeKindsContentProvider("mdsmith", "/repo", spawn);
    const uri = buildResolveUri("/repo/a.md");
    const content = await provider.provideTextDocumentContent(uri);
    expect(content).toContain("~~~json");
    expect(content).toContain('{"file":"a.md"}');
  });
});

describe("runKindsResolve", () => {
  test("opens a resolve virtual doc for the active file", async () => {
    const { deps, openedUris } = makeDeps();
    await runKindsResolve(deps);
    expect(openedUris).toHaveLength(1);
    expect(openedUris[0]).toContain("mdsmith-kinds://resolve");
    expect(openedUris[0]).toContain(encodeURIComponent("/repo/doc.md"));
  });

  test("shows error when no active file", async () => {
    const { deps, errors } = makeDeps({ getActiveFilePath: () => undefined });
    await runKindsResolve(deps);
    expect(errors).toHaveLength(1);
    expect(errors[0]).toContain("Markdown file");
  });
});

describe("runKindsWhy", () => {
  test("picks a rule and opens a why virtual doc", async () => {
    const { deps, openedUris } = makeDeps({
      getDiagnostics: () => [
        { source: "mdsmith", code: "MDS001" },
      ],
    });
    await runKindsWhy(deps);
    expect(openedUris).toHaveLength(1);
    expect(openedUris[0]).toContain("mdsmith-kinds://why");
    expect(openedUris[0]).toContain("rule=MDS001");
  });

  test("returns without opening when rule pick is cancelled", async () => {
    const { deps, openedUris } = makeDeps({
      pickRule: async () => undefined,
    });
    await runKindsWhy(deps);
    expect(openedUris).toHaveLength(0);
  });

  test("shows error when no active file", async () => {
    const { deps, errors } = makeDeps({ getActiveFilePath: () => undefined });
    await runKindsWhy(deps);
    expect(errors).toHaveLength(1);
  });
});
