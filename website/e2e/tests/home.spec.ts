import { test, expect } from "@playwright/test";

/**
 * Homepage positioning and audience-path tests.
 *
 * Covers the category statement (the content/_index.md body that
 * layouts/index.html renders under the hero) and the markdownlint
 * migration link in the hero — the two elements that tell a
 * first-time visitor what mdsmith is and where to start.
 */
test.describe("homepage positioning", () => {
  test("category statement renders under the hero", async ({ page }) => {
    await page.goto("/");

    const positioning = page.locator(".positioning-body");
    await expect(positioning).toBeVisible();
    await expect(positioning).toContainText(
      "mdsmith is a Markdown linter and formatter written in Go",
    );
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
