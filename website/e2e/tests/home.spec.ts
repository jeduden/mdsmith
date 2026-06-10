import { test, expect } from "@playwright/test";

/**
 * Homepage positioning and audience-path tests.
 *
 * Covers the positioning band under the hero (the scope statement
 * from the content/_index.md body plus the one-engine surface row
 * from its front matter) and the markdownlint migration link in
 * the hero — the elements that tell a first-time visitor what
 * mdsmith is and where to start.
 */
test.describe("homepage positioning", () => {
  test("scope statement renders under the hero", async ({ page }) => {
    await page.goto("/");

    const statement = page.locator(".positioning-statement");
    await expect(statement).toBeVisible();
    await expect(statement).toContainText(
      "mdsmith checks style, readability, structure, and cross-file integrity",
    );
  });

  test("runs-in row links each product surface", async ({ page }) => {
    await page.goto("/");

    const row = page.locator(".positioning-engine");
    await expect(row).toBeVisible();
    // Label states the operational fact ("Runs in"), per the design
    // system's voice rules; the one-engine argument belongs to the
    // "Why mdsmith" lead and its pillar, not to a band-level label.
    await expect(row).toContainText("Runs in");

    // Each surface chip is an audience path into the docs: the row
    // replaces the old prose sentence ("One rule engine powers the
    // CLI, the LSP server, and the VS Code extension …"), so every
    // surface named there must stay reachable as a link.
    const chips = row.locator("a.positioning-surface");
    await expect(chips).toHaveCount(6);
    await expect(chips.first()).toHaveAttribute("href", /\/reference\/cli\/$/);
    await expect(
      row.locator("a.positioning-surface", { hasText: "VS Code" }),
    ).toHaveAttribute("href", /\/guides\/editors\/vscode\/$/);
    await expect(
      row.locator("a.positioning-surface", { hasText: "Claude Code" }),
    ).toHaveAttribute("href", /\/features\/editor-agent-integration\/$/);
  });

  test("hero lead names the product category", async ({ page }) => {
    await page.goto("/");

    const lead = page.locator(".hero-lead");
    await expect(lead).toBeVisible();
    await expect(lead).toContainText("Markdown linter and formatter");
    // The lead's job is category + promise; the concrete scope
    // ("cross-file integrity", auto-fix, …) belongs solely to the
    // positioning statement right below it. Guard against the copy
    // drifting back into a feature enumeration that duplicates it.
    await expect(lead).not.toContainText("cross-file integrity");
  });

  test("hero links markdownlint users to the migration guide", async ({
    page,
  }) => {
    await page.goto("/");

    const link = page.locator(".hero-switch a");
    await expect(link).toBeVisible();
    await expect(link).toHaveAttribute(
      "href",
      /\/guides\/migrate-from-markdownlint\/$/,
    );
  });

  test("failed badge images hide their links instead of breaking", async ({
    page,
  }) => {
    // Simulate the badge hosts being blocked or down: abort every
    // request that leaves the local site. The JS in baseof.html
    // must hide each failed badge's link so the hero never shows
    // a broken-image icon.
    await page.route(/^https?:\/\/(?!localhost)/, route => route.abort());
    await page.goto("/");

    const badges = page.locator(".hero-badges a");
    const count = await badges.count();
    expect(count).toBeGreaterThan(0);
    for (let i = 0; i < count; i++) {
      await expect(badges.nth(i)).toBeHidden();
    }
  });

  test("install commands stay readable on a narrow viewport", async ({
    page,
  }) => {
    // Below 560 px each install row wraps: the command line gets the
    // full row width instead of the truncated leftover next to the
    // label and copy button.
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/");

    const row = page.locator(".install-row").first();
    const label = row.locator(".install-label");
    const cmd = row.locator(".install-cmd");
    const rowBox = await row.boundingBox();
    const labelBox = await label.boundingBox();
    const cmdBox = await cmd.boundingBox();
    expect(rowBox).not.toBeNull();
    expect(labelBox).not.toBeNull();
    expect(cmdBox).not.toBeNull();
    // Wrapped: the command renders on a line below the label
    // rather than truncated beside it.
    expect(cmdBox!.y).toBeGreaterThanOrEqual(labelBox!.y + labelBox!.height);
    // Full-width line: the command spans (almost) the row's inner
    // width rather than the ~40% it got next to the label.
    expect(cmdBox!.width).toBeGreaterThan(rowBox!.width * 0.8);
  });
});
