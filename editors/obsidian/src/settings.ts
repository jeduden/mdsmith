// Plugin settings — typed shape, validation, and the change
// classifier main.ts consults to decide whether a settings flush
// needs a server restart or just a listener reconfigure.
//
// Persistence runs through Obsidian's `loadData` / `saveData` (one
// JSON blob per plugin id). The PluginSettingTab subclass lives at
// the bottom of this file so the typed surface stays separate from
// the UI wiring.

import { App, PluginSettingTab, Setting } from "obsidian";

// RunMode picks when the linter runs. `onSave` is the default —
// matching the VS Code extension — because publishDiagnostics
// after every keystroke is overkill for prose.
export type RunMode = "onType" | "onSave" | "off";

// TraceServer controls the LSP trace verbosity. `off` is silent;
// `messages` logs RPC method + id; `verbose` logs the full payload.
// Mirrors mdsmith.trace.server in the VS Code extension.
export type TraceServer = "off" | "messages" | "verbose";

// MdsmithSettings is the typed settings shape. Persisted as JSON.
export interface MdsmithSettings {
  binaryPath: string;
  configPath: string;
  runMode: RunMode;
  fixOnSave: boolean;
  traceServer: TraceServer;
}

// DEFAULTS pins the documented defaults from plan/214 §Settings.
// Changing a default later is a behavior change for every user
// whose stored settings are still empty; do it deliberately.
export const DEFAULTS: MdsmithSettings = {
  binaryPath: "",
  configPath: "",
  runMode: "onSave",
  fixOnSave: false,
  traceServer: "off",
};

const RUN_MODES: readonly RunMode[] = ["onType", "onSave", "off"];
const TRACE_MODES: readonly TraceServer[] = ["off", "messages", "verbose"];

function asString(v: unknown): string | undefined {
  return typeof v === "string" ? v : undefined;
}

function asBool(v: unknown): boolean | undefined {
  if (typeof v === "boolean") return v;
  if (typeof v === "string") {
    if (v === "true") return true;
    if (v === "false") return false;
  }
  if (typeof v === "number") return v !== 0;
  return undefined;
}

function asEnum<T extends string>(v: unknown, allowed: readonly T[]): T | undefined {
  return typeof v === "string" && (allowed as readonly string[]).includes(v)
    ? (v as T)
    : undefined;
}

// normalize takes whatever loadData() returned (possibly null,
// possibly a partial object, possibly a hand-edited file with
// junk values) and produces a clean MdsmithSettings. Unknown keys
// are dropped so saveData round-trips minimal JSON.
export function normalize(raw: unknown): MdsmithSettings {
  const src = (raw && typeof raw === "object" ? raw : {}) as Record<
    string,
    unknown
  >;
  return {
    binaryPath: asString(src.binaryPath) ?? DEFAULTS.binaryPath,
    configPath: asString(src.configPath) ?? DEFAULTS.configPath,
    runMode: asEnum<RunMode>(src.runMode, RUN_MODES) ?? DEFAULTS.runMode,
    fixOnSave: asBool(src.fixOnSave) ?? DEFAULTS.fixOnSave,
    traceServer:
      asEnum<TraceServer>(src.traceServer, TRACE_MODES) ?? DEFAULTS.traceServer,
  };
}

// Reaction is what main.ts should do after a settings flush.
//   - "restart": the binary or config path moved; spawn a fresh
//                server and rewire diagnostics.
//   - "reconfigure": only the runtime knobs changed; reattach
//                listeners without restarting the server.
//   - "none": nothing changed.
export type Reaction = "restart" | "reconfigure" | "none";

// classifyChange picks the heaviest reaction the diff requires. A
// restart subsumes reconfiguring (the new server will pick up the
// fresh listeners on first dispatch).
export function classifyChange(
  before: MdsmithSettings,
  after: MdsmithSettings,
): Reaction {
  if (
    before.binaryPath !== after.binaryPath ||
    before.configPath !== after.configPath
  ) {
    return "restart";
  }
  if (
    before.runMode !== after.runMode ||
    before.fixOnSave !== after.fixOnSave ||
    before.traceServer !== after.traceServer
  ) {
    return "reconfigure";
  }
  return "none";
}

// SettingsHost is the structural subset of the plugin main.ts
// passes into the settings tab. Defined here so the tab does not
// depend on the full plugin class, and so tests can drive the tab
// with a plain object.
export interface SettingsHost {
  settings: MdsmithSettings;
  saveSettings(next: MdsmithSettings): Promise<void>;
}

// MdsmithSettingTab renders the five controls under
// Settings > Community plugins > mdsmith. Each control writes
// back through saveSettings, which round-trips the JSON to disk
// and runs the change classifier.
export class MdsmithSettingTab extends PluginSettingTab {
  constructor(
    app: App,
    private readonly host: SettingsHost & { app: App; manifest: unknown },
  ) {
    super(app, host as unknown as Parameters<typeof PluginSettingTab>[1]);
  }

  override display(): void {
    const container = this.containerEl as unknown as {
      empty?: () => void;
      createEl?: (name: string, opts?: { text?: string }) => unknown;
    };
    if (typeof container.empty === "function") container.empty();
    if (typeof container.createEl === "function") {
      container.createEl("h2", { text: "mdsmith" });
    }

    const write = async (patch: Partial<MdsmithSettings>): Promise<void> => {
      const next: MdsmithSettings = { ...this.host.settings, ...patch };
      await this.host.saveSettings(next);
    };

    new Setting(this.containerEl)
      .setName("Binary path")
      .setDesc(
        "Override the bundled mdsmith binary. Leave empty to use the " +
          "binary shipped with the release zip.",
      )
      .addText((text: { setValue(v: string): unknown; onChange(cb: (v: string) => void): unknown }) => {
        text.setValue(this.host.settings.binaryPath);
        text.onChange(async (v) => {
          await write({ binaryPath: v });
        });
      });

    new Setting(this.containerEl)
      .setName("Config path")
      .setDesc("Override the .mdsmith.yml path (pass `-c <path>` to the server).")
      .addText((text: { setValue(v: string): unknown; onChange(cb: (v: string) => void): unknown }) => {
        text.setValue(this.host.settings.configPath);
        text.onChange(async (v) => {
          await write({ configPath: v });
        });
      });

    new Setting(this.containerEl)
      .setName("Run mode")
      .setDesc("When to re-lint: on every save, on every keystroke, or never.")
      .addDropdown(
        (dd: {
          addOption(value: string, label: string): unknown;
          setValue(v: string): unknown;
          onChange(cb: (v: string) => void): unknown;
        }) => {
          dd.addOption("onSave", "On save");
          dd.addOption("onType", "On type");
          dd.addOption("off", "Off");
          dd.setValue(this.host.settings.runMode);
          dd.onChange(async (v) => {
            await write({ runMode: (v as RunMode) ?? DEFAULTS.runMode });
          });
        },
      );

    new Setting(this.containerEl)
      .setName("Fix on save")
      .setDesc("Run `mdsmith: Fix file` 200 ms after each save.")
      .addToggle(
        (tg: { setValue(v: boolean): unknown; onChange(cb: (v: boolean) => void): unknown }) => {
          tg.setValue(this.host.settings.fixOnSave);
          tg.onChange(async (v) => {
            await write({ fixOnSave: v });
          });
        },
      );

    new Setting(this.containerEl)
      .setName("Trace server")
      .setDesc("LSP trace verbosity. Output appears in the mdsmith console.")
      .addDropdown(
        (dd: {
          addOption(value: string, label: string): unknown;
          setValue(v: string): unknown;
          onChange(cb: (v: string) => void): unknown;
        }) => {
          dd.addOption("off", "Off");
          dd.addOption("messages", "Messages");
          dd.addOption("verbose", "Verbose");
          dd.setValue(this.host.settings.traceServer);
          dd.onChange(async (v) => {
            await write({
              traceServer: (v as TraceServer) ?? DEFAULTS.traceServer,
            });
          });
        },
      );
  }
}
