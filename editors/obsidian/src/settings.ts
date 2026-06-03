// Plugin settings — typed shape, validation, the change classifier
// main.ts consults to decide whether a settings flush needs a runtime
// restart or just a listener reconfigure, and the PluginSettingTab UI.
//
// Plan 217 §Settings declares three controls: configPath, runMode,
// fixOnSave. Persistence runs through Obsidian's loadData / saveData
// (one JSON blob per plugin id).

import { App, PluginSettingTab, Setting } from "obsidian";

// RunMode picks when the linter runs. onSave is the default — matching
// the plan and avoiding a re-check on every keystroke for prose.
export type RunMode = "onType" | "onSave" | "off";

// MdsmithSettings is the typed settings shape, persisted as JSON.
export interface MdsmithSettings {
  // configPath overrides the auto-discovered .mdsmith.yml. Empty defers
  // to the engine's default. Changing it rebuilds the session (plan 215
  // has no in-place reconfigure).
  configPath: string;
  runMode: RunMode;
  // fixOnSave runs Fix file 200 ms after each save when true.
  fixOnSave: boolean;
}

// DEFAULTS pins the documented defaults from plan 217 §Settings.
export const DEFAULTS: MdsmithSettings = {
  configPath: "",
  runMode: "onSave",
  fixOnSave: false,
};

const RUN_MODES: readonly RunMode[] = ["onType", "onSave", "off"];

function asString(v: unknown): string | undefined {
  return typeof v === "string" ? v : undefined;
}

function asBool(v: unknown): boolean | undefined {
  if (typeof v === "boolean") return v;
  if (v === "true") return true;
  if (v === "false") return false;
  if (typeof v === "number") return v !== 0;
  return undefined;
}

function asEnum<T extends string>(
  v: unknown,
  allowed: readonly T[],
): T | undefined {
  return typeof v === "string" && (allowed as readonly string[]).includes(v)
    ? (v as T)
    : undefined;
}

// coerceRunMode validates an arbitrary value — a stored JSON field or a
// UI dropdown selection — against the allowed run modes, falling back to
// the default for anything unexpected (so an out-of-set value can never
// be persisted as the run mode).
export function coerceRunMode(v: unknown): RunMode {
  return asEnum<RunMode>(v, RUN_MODES) ?? DEFAULTS.runMode;
}

// normalize takes whatever loadData() returned (null, a partial object,
// or a hand-edited file with junk values) and produces a clean
// MdsmithSettings. Unknown keys are dropped so saveData round-trips
// minimal JSON — including any leftover keys from the plan-214 LSP
// build (binaryPath, traceServer).
export function normalize(raw: unknown): MdsmithSettings {
  const src = (raw && typeof raw === "object" ? raw : {}) as Record<
    string,
    unknown
  >;
  return {
    configPath: asString(src.configPath) ?? DEFAULTS.configPath,
    runMode: coerceRunMode(src.runMode),
    fixOnSave: asBool(src.fixOnSave) ?? DEFAULTS.fixOnSave,
  };
}

// Reaction is what main.ts should do after a settings flush:
//   - "restart": configPath moved; dispose the session and build a
//                fresh one over the new config.
//   - "reconfigure": only runtime knobs changed; reattach listeners.
//   - "none": nothing changed.
export type Reaction = "restart" | "reconfigure" | "none";

// classifyChange picks the heaviest reaction the diff requires. A
// restart subsumes a reconfigure (the fresh session picks up new
// listeners on first dispatch).
export function classifyChange(
  before: MdsmithSettings,
  after: MdsmithSettings,
): Reaction {
  if (before.configPath !== after.configPath) return "restart";
  if (before.runMode !== after.runMode || before.fixOnSave !== after.fixOnSave) {
    return "reconfigure";
  }
  return "none";
}

// SettingsHost is the structural subset of the plugin the settings tab
// needs. Defined here so the tab does not depend on the full plugin
// class and tests can drive it with a plain object.
export interface SettingsHost {
  settings: MdsmithSettings;
  saveSettings(next: MdsmithSettings): Promise<void>;
}

// MdsmithSettingTab renders the three controls under Settings >
// Community plugins > mdsmith. Each writes back through saveSettings,
// which persists the JSON and runs the change classifier.
export class MdsmithSettingTab extends PluginSettingTab {
  constructor(
    app: App,
    private readonly host: SettingsHost & { app: App; manifest: unknown },
  ) {
    super(app, host as unknown as ConstructorParameters<typeof PluginSettingTab>[1]);
  }

  override display(): void {
    const container = this.containerEl as unknown as {
      empty?: () => void;
    };
    if (typeof container.empty === "function") container.empty();

    const write = async (patch: Partial<MdsmithSettings>): Promise<void> => {
      await this.host.saveSettings({ ...this.host.settings, ...patch });
    };

    new Setting(this.containerEl)
      .setName("Config path")
      .setDesc(
        "Override the auto-discovered .mdsmith.yml. Empty uses the engine " +
          "default. Changing this rebuilds the lint session.",
      )
      .addText(
        (text: {
          setValue(v: string): unknown;
          onChange(cb: (v: string) => void): unknown;
        }) => {
          text.setValue(this.host.settings.configPath);
          text.onChange(async (v) => {
            await write({ configPath: v });
          });
        },
      );

    new Setting(this.containerEl)
      .setName("Run mode")
      .setDesc("When to re-lint: on every keystroke, on save, or never.")
      .addDropdown(
        (dd: {
          addOption(value: string, label: string): unknown;
          setValue(v: string): unknown;
          onChange(cb: (v: string) => void): unknown;
        }) => {
          dd.addOption("onType", "On type");
          dd.addOption("onSave", "On save");
          dd.addOption("off", "Off");
          dd.setValue(this.host.settings.runMode);
          dd.onChange(async (v) => {
            await write({ runMode: coerceRunMode(v) });
          });
        },
      );

    new Setting(this.containerEl)
      .setName("Fix on save")
      .setDesc("Run `mdsmith: Fix file` 200 ms after each save.")
      .addToggle(
        (tg: {
          setValue(v: boolean): unknown;
          onChange(cb: (v: boolean) => void): unknown;
        }) => {
          tg.setValue(this.host.settings.fixOnSave);
          tg.onChange(async (v) => {
            await write({ fixOnSave: v });
          });
        },
      );
  }
}
