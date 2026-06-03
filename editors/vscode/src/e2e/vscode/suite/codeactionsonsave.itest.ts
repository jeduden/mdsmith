// Regression guard for the fix-on-save preview pipeline. Fix-on-save is wired
// the ESLint way — the user puts source.fixAll.mdsmith in VS Code's native
// editor.codeActionsOnSave; the extension contributes only the LSP code
// action. This drives a real (headless) VS Code and proves, end to end:
//   - previewFix off => saving applies the fix silently;
//   - previewFix on  => saving is intercepted by the Refactor Preview (the
//     diff): the save does not complete and the buffer is left untouched until
//     the user confirms.
//
// This is the path a custom willSave handler could not serve (it drops the
// ChangeAnnotation and cannot host a confirmation UI). See plan 207 and
// editors/vscode/src/extension.ts.
import * as assert from "assert";
import * as fs from "fs";
import * as path from "path";
import * as vscode from "vscode";

const WS = vscode.ConfigurationTarget.Workspace;

function settle(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

async function waitForServer(uri: vscode.Uri, timeoutMs = 30000): Promise<number> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    const n = vscode.languages.getDiagnostics(uri).length;
    if (n > 0) return n;
    await settle(250);
  }
  return vscode.languages.getDiagnostics(uri).length;
}

// Save a freshly-dirtied, fixable doc; report whether the save applied the fix
// (no preview) or was intercepted (preview pending). Assumes a warm server.
async function saveOutcome(name: string): Promise<{ outcome: string; changed: boolean }> {
  const root = vscode.workspace.workspaceFolders![0].uri.fsPath;
  const uri = vscode.Uri.file(path.join(root, name));
  fs.writeFileSync(uri.fsPath, "# Title\n\nclean\n");
  const doc = await vscode.workspace.openTextDocument(uri);
  const editor = await vscode.window.showTextDocument(doc);
  await settle(800);
  const edited = await editor.edit((b) =>
    b.insert(new vscode.Position(2, 5), "   ")
  ); // dirty + fixable
  assert.ok(edited, "test setup: editor.edit should succeed and dirty the buffer");
  await settle(400);
  const before = doc.getText();
  // Capture the save promise with a rejection handler *before* racing it. When
  // previewFix intercepts the save, the timeout wins and this promise stays
  // pending — and may later reject when we dismiss the preview below. Swallowing
  // it here keeps a late rejection from surfacing as an unhandled rejection and
  // flaking the run.
  const saved = Promise.resolve(doc.save()).then(
    (ok) => (ok ? "saved" : "save-false"),
    () => "rejected"
  );
  const outcome = await Promise.race([
    saved,
    settle(4000).then(() => "timeout" as const),
  ]);
  const changed = doc.getText() !== before;
  for (const cmd of ["refactorPreview.dismiss", "workbench.action.closePanel"]) {
    await vscode.commands.executeCommand(cmd).then(undefined, () => {});
  }
  return { outcome, changed };
}

suite("editor.codeActionsOnSave + mdsmith.previewFix", () => {
  suiteSetup(async () => {
    await vscode.extensions.getExtension("jeduden.mdsmith")?.activate();
  });

  test("previewFix gates whether a save shows the diff", async () => {
    // User-style wiring: the native setting, exactly like source.fixAll.eslint.
    await vscode.workspace
      .getConfiguration("editor")
      .update("codeActionsOnSave", { "source.fixAll.mdsmith": "explicit" }, WS);
    const mdsmith = vscode.workspace.getConfiguration("mdsmith");
    await mdsmith.update("run", "onType", WS);
    await mdsmith.update("previewFix", false, WS);
    await settle(1500);

    // Warm the server so the saves below don't wait on first-lint.
    const root = vscode.workspace.workspaceFolders![0].uri.fsPath;
    const warm = vscode.Uri.file(path.join(root, `warm-${Date.now()}.md`));
    fs.writeFileSync(warm.fsPath, "# Title\n\nhello   \n");
    await vscode.window.showTextDocument(await vscode.workspace.openTextDocument(warm));
    assert.ok((await waitForServer(warm)) > 0, "server must produce diagnostics");

    // previewFix off -> save applies the fix.
    const off = await saveOutcome(`off-${Date.now()}.md`);
    console.log(`[CAOS off] outcome=${off.outcome} changed=${off.changed}`);
    assert.strictEqual(off.outcome, "saved", "previewFix off: save should complete");
    assert.strictEqual(off.changed, true, "previewFix off: fix should apply on save");

    // previewFix on -> save is intercepted by the Refactor Preview.
    await mdsmith.update("previewFix", true, WS);
    await settle(3000); // forwarder -> didChangeConfiguration -> server re-pull
    const on = await saveOutcome(`on-${Date.now()}.md`);
    console.log(`[CAOS on] outcome=${on.outcome} changed=${on.changed}`);
    assert.strictEqual(on.outcome, "timeout", "previewFix on: save must wait on the preview");
    assert.strictEqual(on.changed, false, "previewFix on: buffer must be untouched until confirmed");
  });
});
