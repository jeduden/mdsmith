// Handler logic for mdsmith.kinds.why and mdsmith.kinds.resolve.
// Both commands open a read-only virtual document populated from the
// JSON output of the corresponding kinds subcommand.

import { SpawnFn, defaultSpawn } from "./runner";
import { buildResolveUri, buildWhyUri, fetchKindsContent } from "./virtual-doc";

export interface DiagnosticLike {
  code?: string | number | { value: string | number; target: unknown };
  source?: string;
}

// extractRuleIds pulls the mdsmith rule IDs (e.g. "MDS001") out of an
// array of VS Code diagnostic objects. Only diagnostics whose source is
// "mdsmith" and whose code is a plain string are included.
export function extractRuleIds(diagnostics: DiagnosticLike[]): string[] {
  const ids = new Set<string>();
  for (const d of diagnostics) {
    if (d.source !== "mdsmith") continue;
    if (typeof d.code === "string" && d.code) ids.add(d.code);
  }
  return Array.from(ids).sort();
}

// KindsResolveHandlerDeps covers the resolve command, which needs no rule picker.
// binary/workspaceRoot/spawn are NOT included here — they belong to
// makeKindsContentProvider, which owns the spawning.
export interface KindsResolveHandlerDeps {
  getActiveFilePath: () => string | undefined;
  getDiagnostics: (filePath: string) => DiagnosticLike[];
  openVirtualDoc: (uri: string) => Promise<void>;
  showError: (msg: string) => Promise<void>;
}

// KindsWhyHandlerDeps extends resolve deps with a rule picker for the why command.
export interface KindsWhyHandlerDeps extends KindsResolveHandlerDeps {
  pickRule: (rules: string[]) => Promise<string | undefined>;
}

export async function runKindsResolve(deps: KindsResolveHandlerDeps): Promise<void> {
  const filePath = deps.getActiveFilePath();
  if (!filePath) {
    await deps.showError("mdsmith: Open a Markdown file first.");
    return;
  }

  const uri = buildResolveUri(filePath);
  await deps.openVirtualDoc(uri);
}

export async function runKindsWhy(deps: KindsWhyHandlerDeps): Promise<void> {
  const filePath = deps.getActiveFilePath();
  if (!filePath) {
    await deps.showError("mdsmith: Open a Markdown file first.");
    return;
  }

  const diagnostics = deps.getDiagnostics(filePath);
  const ruleIds = extractRuleIds(diagnostics);

  const rule = await deps.pickRule(ruleIds);
  if (!rule) return;

  const uri = buildWhyUri(filePath, rule);
  await deps.openVirtualDoc(uri);
}

// KindsContentProvider is the structural interface for a VS Code
// TextDocumentContentProvider. Defined here so extension.ts can
// implement it without importing vscode in this module.
export interface KindsContentProvider {
  provideTextDocumentContent(uri: string): Promise<string>;
}

// makeKindsContentProvider returns a content provider that fetches
// kinds output for the given binary and workspace. The spawn function
// is injectable for testing.
export function makeKindsContentProvider(
  binary: string,
  workspaceRoot: string | undefined,
  spawn: SpawnFn = defaultSpawn
): KindsContentProvider {
  return {
    async provideTextDocumentContent(uri: string): Promise<string> {
      return fetchKindsContent(uri, binary, workspaceRoot, spawn);
    },
  };
}
