// Vault snapshot and change fan-out for the Obsidian plugin.
//
// The WASM session's MemWorkspace is built once at construction from a
// flat snapshot (path → text). Because it is materialized once, a vault
// edit must be pushed in: WorkspaceSync subscribes to the vault's
// modify/create/delete events and forwards each to runtime.invalidate,
// debounced 200 ms per file so a burst of saves collapses to one push.
//
// Obsidian coupling is kept to the structural VaultLike interface so
// this module is testable without the host.

// InvalidateTarget is the slice of the WASM runtime WorkspaceSync
// drives — just invalidate. Narrowing to one method keeps the sync
// decoupled from check/fix.
export interface InvalidateTarget {
  invalidate(uri: string, content?: string): void;
}

// VaultFileLike is the subset of Obsidian's TFile/TAbstractFile the
// snapshot and the change handlers read. extension is optional because
// vault events fire for folders too (a TAbstractFile has no extension);
// the change handler guards on it being "md".
export interface VaultFileLike {
  path: string;
  extension?: string;
}

// EventRef mirrors Obsidian's opaque event handle, returned by on() and
// passed to the plugin's registerEvent for automatic teardown.
export type EventRef = unknown;

// VaultLike is the structural subset of Obsidian's Vault this module
// uses. The real app.vault satisfies it.
export interface VaultLike {
  getMarkdownFiles(): VaultFileLike[];
  // cachedRead returns the file's current text. Obsidian serves it from
  // its in-memory cache, so it is cheap to call on every save.
  cachedRead(file: { path: string }): Promise<string>;
  on(
    event: "modify" | "create" | "delete",
    callback: (file: VaultFileLike) => unknown,
  ): EventRef;
  // offref removes a listener registered by on(), mirroring Obsidian's
  // Events.offref. WorkspaceSync.stop() calls it for every ref start()
  // produced so a restart unsubscribes cleanly.
  offref(ref: EventRef): void;
}

// DEBOUNCE_MS is the per-file fan-out delay from plan 217. A save burst
// (autosave + manual save) within this window collapses to one push.
export const DEBOUNCE_MS = 200;

// snapshotVault reads every Markdown file into the flat map the runtime
// materializes into its MemWorkspace. Keys are vault-relative,
// slash-separated paths — exactly the URIs check/fix/invalidate use.
export async function snapshotVault(
  vault: VaultLike,
): Promise<Record<string, string>> {
  const files = vault.getMarkdownFiles();
  const out: Record<string, string> = {};
  // Read in parallel; the vault serves these from its cache.
  await Promise.all(
    files.map(async (f) => {
      out[f.path] = await vault.cachedRead(f);
    }),
  );
  return out;
}

// WorkspaceSync wires vault events to runtime.invalidate. Construct it
// with the vault, the runtime, and (optionally) a debounce override for
// tests. start() subscribes; stop() unsubscribes via the host and
// cancels any pending push.
export class WorkspaceSync {
  // One trailing timer per changed file. A new event for a file resets
  // its timer; only the last event in the window fires.
  private timers = new Map<string, ReturnType<typeof setTimeout>>();
  private refs: EventRef[] = [];

  constructor(
    private readonly vault: VaultLike,
    private readonly runtime: InvalidateTarget,
    private readonly debounceMs: number = DEBOUNCE_MS,
  ) {}

  // start subscribes to the three vault events and records the EventRef
  // handles so stop() can unsubscribe them. It also returns them for a
  // caller that wants the handles. Each handler debounces per file.
  start(): EventRef[] {
    this.refs = [
      this.vault.on("modify", (f) => this.schedule(f, "upsert")),
      this.vault.on("create", (f) => this.schedule(f, "upsert")),
      this.vault.on("delete", (f) => this.schedule(f, "delete")),
    ];
    return this.refs;
  }

  // schedule (re)arms the trailing timer for one file. An "upsert" reads
  // the latest bytes inside the timer (so the push carries the final
  // content of a burst); a "delete" pushes no content to drop the file.
  private schedule(file: VaultFileLike, kind: "upsert" | "delete"): void {
    // Only Markdown files are in the session's workspace; ignore folder
    // and attachment events.
    if (file.extension !== "md") return;
    const uri = file.path;
    const existing = this.timers.get(uri);
    if (existing) clearTimeout(existing);
    const timer = setTimeout(() => {
      this.timers.delete(uri);
      void this.flush(uri, kind);
    }, this.debounceMs);
    this.timers.set(uri, timer);
  }

  // flush performs the deferred invalidate. For an upsert it reads the
  // current bytes at fire time; a read failure (file vanished between
  // the event and the timer) degrades to a delete so the session never
  // holds stale bytes.
  private async flush(uri: string, kind: "upsert" | "delete"): Promise<void> {
    if (kind === "delete") {
      this.runtime.invalidate(uri);
      return;
    }
    try {
      const content = await this.vault.cachedRead({ path: uri });
      this.runtime.invalidate(uri, content);
    } catch {
      this.runtime.invalidate(uri);
    }
  }

  // stop unsubscribes the vault listeners start() registered and cancels
  // every pending push, so a disposed runtime never receives a late
  // invalidate and a restart does not accumulate duplicate listeners.
  stop(): void {
    for (const ref of this.refs) this.vault.offref(ref);
    this.refs = [];
    for (const timer of this.timers.values()) clearTimeout(timer);
    this.timers.clear();
  }
}
