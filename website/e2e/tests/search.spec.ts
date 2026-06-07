import { test, expect } from "@playwright/test";

/**
 * ⌘K documentation search end-to-end tests.
 *
 * Covers the interactive JavaScript in static/js/search.js that a
 * static render cannot reach: the keyboard shortcut, the dialog
 * lifecycle, querying the JSON index, keyboard navigation, and the
 * no-JS fallback (the trigger is hidden, the sidebar still navigates).
 *
 * The shortcut is exercised with Control+K on every platform: search.js
 * opens on `e.metaKey || e.ctrlKey`, so Ctrl works in the headless
 * browser regardless of the host OS (no Node `process` platform sniff).
 */

test.describe("⌘K search", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
  });

  // ─── index ─────────────────────────────────────────────────────────

  test("the JSON search index is published and well-formed", async ({
    page,
  }) => {
    const res = await page.request.get("/index.json");
    expect(res.ok()).toBeTruthy();
    const docs = await res.json();
    expect(Array.isArray(docs)).toBeTruthy();
    expect(docs.length).toBeGreaterThan(20);
    for (const key of ["title", "summary", "section", "href", "body"]) {
      expect(docs[0]).toHaveProperty(key);
    }
  });

  // ─── opening ────────────────────────────────────────────────────────

  test("the trigger button is visible and opens the dialog", async ({
    page,
  }) => {
    const trigger = page.locator("[data-search-open]");
    await expect(trigger).toBeVisible();

    const dialog = page.locator("[data-search-dialog]");
    await expect(dialog).toBeHidden();

    await trigger.click();
    await expect(dialog).toBeVisible();
    await expect(page.locator("[data-search-input]")).toBeFocused();
  });

  test("the modifier+K shortcut opens the dialog", async ({ page }) => {
    const dialog = page.locator("[data-search-dialog]");
    await expect(dialog).toBeHidden();

    await page.keyboard.press("Control+k");
    await expect(dialog).toBeVisible();
    await expect(page.locator("[data-search-input]")).toBeFocused();
  });

  test("the modifier+K shortcut toggles the dialog closed again", async ({
    page,
  }) => {
    const dialog = page.locator("[data-search-dialog]");
    await page.keyboard.press("Control+k");
    await expect(dialog).toBeVisible();
    await page.keyboard.press("Control+k");
    await expect(dialog).toBeHidden();
  });

  test("the '/' shortcut opens search when not typing in a field", async ({
    page,
  }) => {
    const dialog = page.locator("[data-search-dialog]");
    await expect(dialog).toBeHidden();
    await page.keyboard.press("/");
    await expect(dialog).toBeVisible();
  });

  // ─── querying ───────────────────────────────────────────────────────

  test("typing a query renders matching results", async ({ page }) => {
    await page.keyboard.press("Control+k");
    const input = page.locator("[data-search-input]");
    await input.fill("auto-fix");

    const results = page.locator("[data-search-results] .search-result");
    await expect(results.first()).toBeVisible();

    // The auto-fix feature page must surface for that query.
    const autofix = page.locator(
      '[data-search-results] a[href="/features/auto-fix/"]'
    );
    await expect(autofix).toBeVisible();
  });

  test("a query with no matches shows the empty status", async ({ page }) => {
    await page.keyboard.press("Control+k");
    await page.locator("[data-search-input]").fill("zzzznomatchquery");

    // Assert the real empty-state text, not merely a visible status: the
    // "Loading…" status is also visible with zero results while the index
    // fetches, so toContainText waits for the index to land and re-render.
    const status = page.locator("[data-search-status]");
    await expect(status).toContainText("No results");
    const results = page.locator("[data-search-results] .search-result");
    await expect(results).toHaveCount(0);
  });

  test("ArrowDown then Enter navigates to the active result", async ({
    page,
  }) => {
    await page.keyboard.press("Control+k");
    await page.locator("[data-search-input]").fill("install");

    // First result is active on render; move to the second, then open.
    const second = page.locator(
      "[data-search-results] .search-result"
    ).nth(1);
    await expect(second).toBeVisible();
    await page.keyboard.press("ArrowDown");
    await expect(second).toHaveClass(/is-active/);

    const href = await second.locator("a").getAttribute("href");
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(new RegExp(escapeRegExp(href!) + "$"));
  });

  test("clicking a result navigates to its page", async ({ page }) => {
    await page.keyboard.press("Control+k");
    await page.locator("[data-search-input]").fill("auto-fix");

    const link = page.locator(
      '[data-search-results] a[href="/features/auto-fix/"]'
    );
    await link.click();
    await expect(page).toHaveURL(/\/features\/auto-fix\/$/);
  });

  // ─── closing ────────────────────────────────────────────────────────

  test("Escape closes the dialog even with a non-empty query", async ({
    page,
  }) => {
    const dialog = page.locator("[data-search-dialog]");
    await page.keyboard.press("Control+k");
    await expect(dialog).toBeVisible();
    // Type first: with a non-empty type=search input some engines (WebKit)
    // consume the first Escape to clear the field, so search.js handles
    // Escape explicitly. One Escape must still close the dialog.
    await page.locator("[data-search-input]").fill("schema");
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
  });

  // ─── no-JS fallback ─────────────────────────────────────────────────

  test("the trigger is hidden when JavaScript is disabled", async ({
    browser,
  }) => {
    const ctx = await browser.newContext({ javaScriptEnabled: false });
    const page = await ctx.newPage();
    await page.goto("/");

    // Without html.js the trigger collapses to display:none; the docs
    // sidebar remains the way to navigate.
    await expect(page.locator("[data-search-open]")).toBeHidden();
    await ctx.close();
  });
});

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
