// Wiring helpers extracted from extension.ts so the spawn/glob/error
// shapes can be unit-tested without booting a real VS Code host.
//
// These functions deliberately depend only on the types from
// `vscode-languageclient/node` plus a couple of structural shapes
// (FileSystemWatcher, Uri-like) so `bun test` can stand in lightweight
// fakes without pulling in the full `vscode` runtime.

import {
  LanguageClientOptions,
  ServerOptions,
  TransportKind
} from "vscode-languageclient/node";

// FileSystemWatcherLike is the structural subset of
// vscode.FileSystemWatcher that LanguageClientOptions.synchronize.fileEvents
// actually consults. Tests can pass a bare object literal.
export interface FileSystemWatcherLike {
  ignoreCreateEvents?: boolean;
  ignoreChangeEvents?: boolean;
  ignoreDeleteEvents?: boolean;
}

export function buildServerOptions(binary: string, transport: TransportKind): ServerOptions {
  return {
    run: { command: binary, args: ["lsp"], transport },
    debug: { command: binary, args: ["lsp"], transport }
  };
}

export function buildClientOptions(
  configWatcher: FileSystemWatcherLike
): LanguageClientOptions {
  return {
    documentSelector: [
      { scheme: "file", language: "markdown" }
    ],
    synchronize: {
      // Cast: LanguageClientOptions wants the full vscode interface,
      // but at runtime only the shape we expose matters.
      fileEvents: configWatcher as never
    },
    outputChannelName: "mdsmith"
  };
}

export function startupErrorMessage(err: unknown): string {
  return (
    `Failed to start mdsmith Language Server: ${err}. ` +
    `Set the binary path with the "mdsmith.path" setting or download mdsmith.`
  );
}

// Minimal shapes of the bits of vscode.CodeAction / WorkspaceEdit /
// Uri / TextEdit we touch when filtering fixAll edits. Defining them
// here lets tests drive the pure pipeline without importing `vscode`.
export interface UriLike {
  toString(): string;
}

export interface TextEditLike {
  // marker — we forward as-is.
}

export interface WorkspaceEditLike {
  entries(): readonly [UriLike, TextEditLike[]][];
}

export interface CodeActionLike {
  kind?: { value: string };
  edit?: WorkspaceEditLike;
}

// collectFixAllEdits filters the array returned by
// `vscode.executeCodeActionProvider` down to the TextEdits a
// fixAll-on-save handler should apply. We keep only edits whose
// CodeAction.kind is exactly "source.fixAll.mdsmith" and whose URI
// matches the document being saved; everything else (other kinds,
// other files, missing edits) is dropped.
export function collectFixAllEdits(
  actions: unknown,
  documentUri: UriLike
): TextEditLike[] {
  const list = (actions ?? []) as CodeActionLike[];
  const target = documentUri.toString();
  const edits: TextEditLike[] = [];
  for (const action of list) {
    if (action.kind?.value !== "source.fixAll.mdsmith") continue;
    if (!action.edit) continue;
    for (const [uri, items] of action.edit.entries()) {
      if (uri.toString() !== target) continue;
      for (const item of items) {
        edits.push(item);
      }
    }
  }
  return edits;
}
