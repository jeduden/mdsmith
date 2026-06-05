// Vault-root config discovery for the Obsidian plugin.
//
// The CLI finds a project's config by walking up from the working
// directory to the nearest .mdsmith.yml (stopping at the repository
// root). An Obsidian vault has a single root the adapter is rooted at,
// and the plugin holds one lint session over the whole vault, so the
// analog is one config file at the vault root. When the Config path
// setting is empty, the plugin reads .mdsmith.yml from there instead of
// falling straight back to the engine defaults.
//
// Obsidian's vault file API (getMarkdownFiles, getAbstractFileByPath)
// hides dotfiles, so .mdsmith.yml never appears there. The DataAdapter
// sees the raw vault directory, so discovery goes through it — the same
// path the explicit-config read uses, which is why it also works on
// mobile.

// CONFIG_FILE_NAME is the config basename the plugin discovers, matching
// the .mdsmith.yml the CLI walks the directory tree for.
export const CONFIG_FILE_NAME = ".mdsmith.yml";

// ConfigAdapter is the slice of Obsidian's DataAdapter discovery needs:
// a vault-relative existence check and text read. The real
// app.vault.adapter satisfies it on desktop and mobile alike.
export interface ConfigAdapter {
  exists(path: string): Promise<boolean>;
  read(path: string): Promise<string>;
}

// discoverConfigYAML returns the text of the vault-root .mdsmith.yml, or
// "" when none is present. A read that fails after the existence check —
// a race with a delete, a permission error — also degrades to "", so a
// missing or unreadable file falls back to the built-in defaults rather
// than throwing. It does NOT validate the contents: a present, readable
// but malformed .mdsmith.yml is the engine's to reject — NewSession
// surfaces the parse error on session creation, exactly as the CLI does
// for a discovered config.
export async function discoverConfigYAML(
  adapter: ConfigAdapter,
  name: string = CONFIG_FILE_NAME,
): Promise<string> {
  try {
    if (!(await adapter.exists(name))) return "";
    return await adapter.read(name);
  } catch {
    return "";
  }
}
