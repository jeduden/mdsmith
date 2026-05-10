// TextDocumentContentProvider for the mdsmith-kinds: URI scheme.
// URIs encode the subcommand and arguments so the provider knows what
// to run without any external state. Closing the tab discards the buffer.
//
// URI format:
//   mdsmith-kinds://resolve?file=<encoded-path>
//   mdsmith-kinds://why?file=<encoded-path>&rule=<rule-id>

import { dirname } from "path";
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

// parseKindsUri extracts command, file, and optional rule from a
// mdsmith-kinds: URI string. Returns null when the URI is malformed.
export function parseKindsUri(uri: string): {
  command: "resolve" | "why";
  file: string;
  rule?: string;
} | null {
  const match = uri.match(new RegExp(`^${KINDS_SCHEME}://(resolve|why)\\?(.+)$`));
  if (!match) return null;
  const command = match[1] as "resolve" | "why";
  const params = new URLSearchParams(match[2]);
  const file = params.get("file");
  if (!file) return null;
  const rule = params.get("rule") ?? undefined;
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
    return `<!-- mdsmith: malformed kinds URI: ${uri} -->`;
  }

  const args =
    parsed.command === "resolve"
      ? ["kinds", "resolve", parsed.file, "--json"]
      : ["kinds", "why", parsed.file, parsed.rule!, "--json"];

  let result: Awaited<ReturnType<typeof spawn>>;
  try {
    result = await spawn(binary, args, workspaceRoot ?? dirname(parsed.file));
  } catch (err) {
    return `**mdsmith ${args.slice(0, 2).join(" ")} could not start**\n\n\`\`\`\n${err}\n\`\`\``;
  }

  if (result.exitCode !== 0) {
    return `**mdsmith ${args.slice(0, 2).join(" ")} failed (exit ${result.exitCode})**\n\n\`\`\`\n${result.stderr.trim()}\n\`\`\``;
  }

  const label =
    parsed.command === "resolve"
      ? `Resolved config: ${parsed.file}`
      : `Rule ${parsed.rule} on ${parsed.file}`;

  return `# ${label}\n\n\`\`\`json\n${result.stdout.trim()}\n\`\`\``;
}
