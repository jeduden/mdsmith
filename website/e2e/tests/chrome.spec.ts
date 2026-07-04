import { test, expect } from "./hermetic";

/**
 * Chrome (browser chrome) end-to-end tests.
 *
 * Covers the scroll-triggered nav state and the docs sidebar toggle —
 * both driven by the JS in baseof.html that the Go render probes
 * cannot exercise.
 */
test.describe("browser chrome", () => {
  // ─── top nav scroll state ─────────────────────────────────────────

  test("scrolling down adds is-scrolled to .topnav", async ({ page }) => {
    await page.goto("/");

    const nav = page.locator(".topnav");
    // At the top the class must not be present (scrollY ≤ 4).
    await expect(nav).not.toHaveClass(/is-scrolled/);

    // Scroll past the 4 px threshold.
    await page.evaluate(() => window.scrollTo(0, 100));

    await expect(nav).toHaveClass(/is-scrolled/);
  });

  test("scrolling back to top removes is-scrolled from .topnav", async ({
    page,
  }) => {
    await page.goto("/");

    const nav = page.locator(".topnav");
    await page.evaluate(() => window.scrollTo(0, 100));
    await expect(nav).toHaveClass(/is-scrolled/);

    await page.evaluate(() => window.scrollTo(0, 0));
    await expect(nav).not.toHaveClass(/is-scrolled/);
  });

  // ─── docs sidebar toggle ─────────────────────────────────────────

  test("docs sidebar toggle adds is-open and sets aria-expanded", async ({
    page,
  }) => {
    // The sidebar toggle is visible only on narrow (≤880 px) viewports.
    // Set a mobile-width viewport so the CSS enables the toggle.
    await page.setViewportSize({ width: 600, height: 800 });

    // Navigate to a docs content page that carries the sidebar and toggle.
    // The auto-fix feature page is a stable, always-rendered example.
    await page.goto("/features/auto-fix/");

    const toggle = page.locator(".docs-side-toggle");
    const sidebar = page.locator("#docs-nav");

    await expect(toggle).toBeVisible();

    // Initial state: collapsed (html.js nav is display:none; not is-open).
    await expect(sidebar).not.toHaveClass(/is-open/);
    await expect(toggle).toHaveAttribute("aria-expanded", "false");

    // Click to open.
    await toggle.click();
    await expect(sidebar).toHaveClass(/is-open/);
    await expect(toggle).toHaveAttribute("aria-expanded", "true");

    // Click to close.
    await toggle.click();
    await expect(sidebar).not.toHaveClass(/is-open/);
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
  });
});
