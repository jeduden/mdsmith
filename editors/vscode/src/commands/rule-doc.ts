// Offline rule documentation: a TextDocumentContentProvider backing for
// the mdsmith-rule: URI scheme plus the hover-link rewrite that points
// VS Code at it instead of the public docs website.
//
// The LSP server emits hover links to <https://mdsmith.dev/rules/…> so
// that *any* editor gets a working link. mdsmith itself never fetches
// that URL — but following it needs a browser and a network. Because the
// full rule README is embedded in the binary and printed offline by
// `mdsmith help rule <id>`, the extension rewrites that link to a
// command: link that opens the README in a read-only virtual document.
// Closing the tab discards the buffer; nothing touches the network.
//
// URI format:
//   mdsmith-rule://doc?id=<RULE-ID>

import { SpawnFn, defaultSpawn } from "./runner";

export const RULE_SCHEME = "mdsmith-rule";

// OPEN_RULE_DOC_COMMAND is the command a rewritten hover link invokes.
// It is registered programmatically (not a palette command) and is the
// only command the rewritten MarkdownString trusts.
export const OPEN_RULE_DOC_COMMAND = "mdsmith.openRuleDoc";

// RULE_ID_RE matches an mdsmith rule ID. Rule IDs are always "MDS"
// followed by digits; `mdsmith help rule` accepts the bare ID (case
// insensitive) but not the docs slug, so the ID is what we extract and
// pass through.
const RULE_ID_RE = /^MDS\d+$/i;

// isRuleId reports whether s is a well-formed mdsmith rule ID. The
// mdsmith-rule: URI, the link rewrite, and the openRuleDoc command all
// gate on this — the single source of truth for the constraint — so only
// a real rule ID ever reaches the spawned `mdsmith help rule`.
export function isRuleId(s: string): boolean {
  return RULE_ID_RE.test(s);
}

// RULE_DOC_LINK_RE matches a Markdown link target that points at a
// published rule-docs page, e.g.
//   ](https://mdsmith.dev/rules/mds020-required-structure/)
// It is host-agnostic — it keys off the "/rules/<id>-<name>/" path so a
// docs-host change does not silently disable the offline rewrite — and
// captures the lower-cased rule ID (group 2) out of the slug prefix.
const RULE_DOC_LINK_RE =
  /\]\((https?:\/\/[^\s)]*\/rules\/(mds\d+)-[a-z0-9-]*\/?)\)/gi;

// buildRuleDocUri returns the virtual-document URI for a rule ID. The ID
// rides in a query parameter (not the hostname) so its case survives URL
// normalization.
export function buildRuleDocUri(id: string): string {
  return `${RULE_SCHEME}://doc?id=${encodeURIComponent(id)}`;
}

// parseRuleDocUri extracts the rule ID from a mdsmith-rule: URI. Returns
// null when the URI is malformed, has the wrong scheme/host, carries no
// id, or the id is not a well-formed rule ID. The last guard is
// defense-in-depth: only "MDS<digits>" ever reaches the spawned binary,
// so a hand-crafted mdsmith-rule:// URI cannot smuggle an arbitrary
// argument into `mdsmith help rule`.
export function parseRuleDocUri(uri: string): { id: string } | null {
  let url: URL;
  try {
    url = new URL(uri);
  } catch {
    return null;
  }
  if (url.protocol !== `${RULE_SCHEME}:`) return null;
  if (url.hostname !== "doc") return null;
  const id = url.searchParams.get("id");
  if (!id || !isRuleId(id)) return null;
  return { id };
}

// ruleDocCommandUri returns the command: URI that opens the offline rule
// doc for an ID, with the ID encoded as the command's single JSON
// argument. Only fires when the hosting MarkdownString trusts
// OPEN_RULE_DOC_COMMAND.
export function ruleDocCommandUri(id: string): string {
  const args = encodeURIComponent(JSON.stringify([id]));
  return `command:${OPEN_RULE_DOC_COMMAND}?${args}`;
}

// rewriteRuleDocLinks swaps every published rule-docs link target in a
// hover markdown string for a command: link that opens the doc offline.
// The link label is preserved. Returns the input unchanged when it holds
// no rule-docs link, so the caller can leave non-rule hovers (and their
// trust level) untouched.
export function rewriteRuleDocLinks(markdown: string): string {
  return markdown.replace(
    RULE_DOC_LINK_RE,
    (_full, _url: string, idLower: string) =>
      `](${ruleDocCommandUri(idLower.toUpperCase())})`,
  );
}

// MarkdownLike is the structural subset of vscode.MarkdownString the
// hover rewrite touches. Defined here so the transform is unit-testable
// without the vscode runtime.
export interface MarkdownLike {
  value: string;
  isTrusted?: boolean | { readonly enabledCommands: readonly string[] };
}

// rewriteHoverMarkdown rewrites rule-docs links in one markdown block
// and, when it changed anything, trusts ONLY OPEN_RULE_DOC_COMMAND so the
// command link is clickable without opening the markdown to arbitrary
// commands. Returns true when the block was rewritten. Blocks with no
// rule-docs link are left untouched — including their isTrusted — so we
// never broaden trust on unrelated hovers.
export function rewriteHoverMarkdown(md: MarkdownLike): boolean {
  const next = rewriteRuleDocLinks(md.value);
  if (next === md.value) return false;
  md.value = next;
  md.isTrusted = { enabledCommands: [OPEN_RULE_DOC_COMMAND] };
  return true;
}

// fetchRuleDocContent runs `mdsmith help rule <id>` and returns the
// embedded README markdown. On error the stderr (or spawn failure) is
// returned as plain text. Fully offline: help rule reads READMEs
// embedded in the binary and makes no network call.
export async function fetchRuleDocContent(
  uri: string,
  binary: string,
  workspaceRoot: string | undefined,
  spawn: SpawnFn = defaultSpawn,
): Promise<string> {
  const parsed = parseRuleDocUri(uri);
  if (!parsed) {
    return `**mdsmith: malformed rule URI**\n\n~~~\n${uri}\n~~~`;
  }

  const args = ["help", "rule", parsed.id];

  let result: Awaited<ReturnType<typeof spawn>>;
  try {
    result = await spawn(binary, args, workspaceRoot);
  } catch (err) {
    return `**mdsmith help rule could not start**\n\n~~~\n${err}\n~~~`;
  }

  if (result.exitCode !== 0) {
    return `**mdsmith help rule ${parsed.id} failed (exit ${result.exitCode})**\n\n~~~\n${result.stderr.trim()}\n~~~`;
  }

  return result.stdout.trimEnd() + "\n";
}
