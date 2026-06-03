// TextDocumentContentProvider for the mdsmith-kinds: URI scheme.
// URIs encode the subcommand and arguments so the provider knows what
// to run without any external state. Closing the tab discards the buffer.
//
// URI format:
//   mdsmith-kinds://resolve?file=<encoded-path>
//   mdsmith-kinds://why?file=<encoded-path>&rule=<rule-id>

import { dirname, isAbsolute } from "node:path";
import { SpawnFn, defaultSpawn } from "./runner";

export const KINDS_SCHEME = "mdsmith-kinds";

// buildResolveUri returns the virtual-document URI for "kinds resolve".
export function buildResolveUri(filePath: string): string {
  return `${KINDS_SCHEME}://resolve?file=${encodeURIComponent(filePath)}`;
}

// buildWhyUri returns the virtual-document URI for "kinds why".
export function buildWhyUri(filePath: string, rule: string): string {
  return (
    `${KINDS_SCHEME}://why` +
    `?file=${encodeURIComponent(filePath)}` +
    `&rule=${encodeURIComponent(rule)}`
  );
}

// kindsContentUri returns the parseable string form of the Uri handed
// to the mdsmith-kinds: TextDocumentContentProvider. Its argument is the
// structural slice of vscode.Uri this needs, declared inline so the
// function stays unit-testable without the editor runtime.
//
// skipEncoding=true is required. VS Code's default Uri.toString()
// percent-encodes the query separators ("=" → %3D, and the param-joining
// "&" → %26), so parseKindsUri can no longer split out the file and rule
// keys and the provider reports "malformed kinds URI". skipEncoding
// leaves those separators intact.
export function kindsContentUri(uri: {
  toString(skipEncoding?: boolean): string;
}): string {
  return uri.toString(true);
}

// parseKindsUri extracts command, file, and optional rule from a
// mdsmith-kinds: URI string. Returns null when the URI is malformed.
// Uses URL parsing so both "mdsmith-kinds://resolve?file=…" and the
// normalized form "mdsmith-kinds://resolve/?file=…" are accepted.
export function parseKindsUri(uri: string): {
  command: "resolve" | "why";
  file: string;
  rule?: string;
} | null {
  let url: URL;
  try {
    url = new URL(uri);
  } catch {
    return null;
  }
  if (url.protocol !== `${KINDS_SCHEME}:`) return null;
  const command = url.hostname;
  if (command !== "resolve" && command !== "why") return null;
  const file = url.searchParams.get("file");
  if (!file) return null;
  const rule = url.searchParams.get("rule") ?? undefined;
  if (command === "why" && !rule) return null;
  return { command, file, rule };
}

// fetchKindsContent runs the appropriate mdsmith kinds subcommand and
// returns the stdout (JSON) rendered as a fenced code block. On error
// the stderr is returned as plain text.
export async function fetchKindsContent(
  uri: string,
  binary: string,
  workspaceRoot: string | undefined,
  spawn: SpawnFn = defaultSpawn
): Promise<string> {
  const parsed = parseKindsUri(uri);
  if (!parsed) {
    return `**mdsmith: malformed kinds URI**\n\n~~~\n${uri}\n~~~`;
  }
  if (!isAbsolute(parsed.file)) {
    return `**mdsmith: kinds URI file path must be absolute**\n\n~~~\n${parsed.file}\n~~~`;
  }

  const args =
    parsed.command === "resolve"
      ? ["kinds", "resolve", "--json", "--", parsed.file]
      : ["kinds", "why", "--json", "--", parsed.file, parsed.rule!];

  let result: Awaited<ReturnType<typeof spawn>>;
  try {
    result = await spawn(binary, args, workspaceRoot ?? dirname(parsed.file));
  } catch (err) {
    return `**mdsmith ${args.slice(0, 2).join(" ")} could not start**\n\n~~~\n${err}\n~~~`;
  }

  if (result.exitCode !== 0) {
    return `**mdsmith ${args.slice(0, 2).join(" ")} failed (exit ${result.exitCode})**\n\n~~~\n${result.stderr.trim()}\n~~~`;
  }

  const label =
    parsed.command === "resolve"
      ? `Resolved config: ${parsed.file}`
      : `Rule ${parsed.rule} on ${parsed.file}`;

  return `# ${label}\n\n~~~json\n${result.stdout.trim()}\n~~~`;
}
