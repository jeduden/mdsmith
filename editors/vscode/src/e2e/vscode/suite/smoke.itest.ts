import * as assert from "assert";
import * as vscode from "vscode";

suite("smoke", () => {
  test("vscode api + mdsmith extension are available", async () => {
    assert.ok(vscode.workspace, "vscode.workspace must exist");
    const ext = vscode.extensions.getExtension("jeduden.mdsmith");
    assert.ok(ext, "mdsmith extension should be discoverable in the host");
    await ext!.activate();
    assert.strictEqual(ext!.isActive, true, "extension should activate");
  });
});
