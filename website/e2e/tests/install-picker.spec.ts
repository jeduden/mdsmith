import { test, expect, blockCrossOrigin } from "./hermetic";

/**
 * Install picker end-to-end tests.
 *
 * Covers the interactive JavaScript that the Go render probes
 * (verifypicker.go) cannot reach: chip filtering, the Windows command
 * swap, copy-to-clipboard feedback, and the no-JS noscript fallback.
 */
test.describe("install picker", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
  });

  // ─── chip filtering ───────────────────────────────────────────────

  test("All chip shows every row", async ({ page }) => {
    // The "All" chip is active by default; all rows must be visible.
    const picker = page.locator("[data-install-picker]");
    const allChip = picker.locator(".install-filter[data-filter=all]");
    await expect(allChip).toHaveAttribute("aria-pressed", "true");

    const rows = picker.locator(".install-row");
    const count = await rows.count();
    expect(count).toBeGreaterThan(0);
    for (let i = 0; i < count; i++) {
      await expect(rows.nth(i)).not.toHaveAttribute("hidden");
    }
  });

  test("platform chip hides rows that lack the tag", async ({ page }) => {
    const picker = page.locator("[data-install-picker]");
    const windowsChip = picker.locator(".install-filter[data-filter=windows]");
    await windowsChip.click();

    await expect(windowsChip).toHaveAttribute("aria-pressed", "true");
    await expect(windowsChip).toHaveClass(/is-active/);

    const rows = picker.locator(".install-row");
    const count = await rows.count();
    for (let i = 0; i < count; i++) {
      const row = rows.nth(i);
      const platforms = await row.getAttribute("data-platforms");
      const tags = (platforms ?? "").split(" ").filter(Boolean);
      if (tags.includes("windows")) {
        await expect(row).not.toHaveAttribute("hidden");
      } else {
        await expect(row).toHaveAttribute("hidden", "");
      }
    }
  });

  test("platform chip hides rows not tagged for that platform", async ({
    page,
  }) => {
    const picker = page.locator("[data-install-picker]");
    const macosChip = picker.locator(".install-filter[data-filter=macos]");
    await macosChip.click();

    const editorRows = picker.locator(
      '.install-row[data-platforms="editor"]'
    );
    const count = await editorRows.count();
    for (let i = 0; i < count; i++) {
      await expect(editorRows.nth(i)).toHaveAttribute("hidden", "");
    }
  });

  // ─── Windows swap ─────────────────────────────────────────────────

  test("Windows chip swaps GitHub Releases row to the .exe command", async ({
    page,
  }) => {
    const picker = page.locator("[data-install-picker]");
    const ghRow = picker.locator(
      ".install-row[data-cmd-windows]"
    );
    const windowsCmd = await ghRow.getAttribute("data-cmd-windows");
    expect(windowsCmd).toBeTruthy();
    expect(windowsCmd).toContain(".exe");

    const windowsChip = picker.locator(".install-filter[data-filter=windows]");
    await windowsChip.click();

    const cmdEl = ghRow.locator(".install-cmd .cmd");
    await expect(cmdEl).toHaveText(windowsCmd!);
  });

  test("All chip restores the default curl command after Windows", async ({
    page,
  }) => {
    const picker = page.locator("[data-install-picker]");
    const ghRow = picker.locator(".install-row[data-cmd-windows]");
    const defaultCmd = await ghRow.getAttribute("data-cmd-default");
    expect(defaultCmd).toBeTruthy();

    // Switch to Windows, then back to All.
    const windowsChip = picker.locator(".install-filter[data-filter=windows]");
    await windowsChip.click();
    const allChip = picker.locator(".install-filter[data-filter=all]");
    await allChip.click();

    const cmdEl = ghRow.locator(".install-cmd .cmd");
    await expect(cmdEl).toHaveText(defaultCmd!);
  });

  // ─── copy-to-clipboard ────────────────────────────────────────────

  test("copy button shows 'copied' then restores after 1.4 s", async ({
    page,
    context,
  }) => {
    // Grant clipboard permissions so writeText and readText work.
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);

    const picker = page.locator("[data-install-picker]");
    const firstCopyBtn = picker.locator(".install-copy").first();
    const originalLabel = await firstCopyBtn.textContent();

    await firstCopyBtn.click();
    await expect(firstCopyBtn).toHaveText("copied");
    await expect(firstCopyBtn).toHaveClass(/is-copied/);

    // After 1.4 s the button should restore to the original label.
    await expect(firstCopyBtn).toHaveText(originalLabel!, { timeout: 3000 });
    await expect(firstCopyBtn).not.toHaveClass(/is-copied/);
  });

  test("copy writes the currently shown command to the clipboard", async ({
    page,
    context,
  }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);

    const picker = page.locator("[data-install-picker]");

    // Switch to Windows so the GitHub row shows the .exe command.
    const windowsChip = picker.locator(".install-filter[data-filter=windows]");
    await windowsChip.click();

    const ghRow = picker.locator(".install-row[data-cmd-windows]");
    const windowsCmd = await ghRow.getAttribute("data-cmd-windows");
    expect(windowsCmd).toBeTruthy();

    const copyBtn = ghRow.locator(".install-copy");
    await copyBtn.click();

    const clipboard = await page.evaluate(() =>
      navigator.clipboard.readText()
    );
    expect(clipboard).toBe(windowsCmd);
  });

  // ─── noscript fallback ────────────────────────────────────────────

  test("noscript Windows command is visible when JS is disabled", async ({
    browser,
  }) => {
    const ctx = await browser.newContext({ javaScriptEnabled: false });
    // With JS disabled the hero's lazy badge images load eagerly and
    // gate the load event on third-party hosts; see hermetic.ts.
    await blockCrossOrigin(ctx);
    const page = await ctx.newPage();
    await page.goto("/");

    // The <noscript> block should be visible and show the Windows .exe line.
    const noscriptCmd = page
      .locator(".install-cmd-noscript")
      .first();
    await expect(noscriptCmd).toBeVisible();
    await expect(noscriptCmd).toContainText(".exe");
    await ctx.close();
  });
});
