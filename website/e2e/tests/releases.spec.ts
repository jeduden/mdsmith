import { test, expect } from "./hermetic";

/**
 * Release-notes page end-to-end tests.
 *
 * serve.sh builds the site without website/data/releases.json (only
 * the deploy runs `mdsmith-release sync-releases`, which needs a
 * GitHub token), so this suite pins the no-data fallback and the
 * footer entry point. The data-driven render is covered by the Go
 * unit tests in internal/release and by pages.yml, which renders and
 * link-probes the page with the real release list.
 */
test.describe("release notes", () => {
  test("footer links to the release-notes page", async ({ page }) => {
    await page.goto("/");
    const link = page.locator("footer.footer").getByRole("link", {
      name: "Release notes",
    });
    await expect(link).toHaveAttribute("href", "/releases/");
    await link.click();
    await expect(page).toHaveURL(/\/releases\/$/);
    await expect(
      page.getByRole("heading", { level: 1, name: "Release notes" }),
    ).toBeVisible();
  });

  test("top-nav version badge opens that version's release notes", async ({
    page,
  }) => {
    await page.goto("/");
    const badge = page.locator(".topnav-version");
    const version = (await badge.textContent())?.trim() ?? "";
    expect(version).toMatch(/^v\d/);
    await expect(badge).toHaveAttribute("href", `/releases/#${version}`);
    await badge.click();
    await expect(page).toHaveURL(new RegExp(`/releases/#${version.replace(/\./g, "\\.")}$`));
  });

  test("footer keeps a direct link to GitHub Releases", async ({ page }) => {
    await page.goto("/");
    await expect(
      page.locator("footer.footer").getByRole("link", { name: "GitHub Releases" }),
    ).toHaveAttribute("href", "https://github.com/jeduden/mdsmith/releases");
  });

  test("without release data the page links to GitHub Releases", async ({
    page,
  }) => {
    await page.goto("/releases/");
    await expect(page.locator(".releases-missing")).toBeVisible();
    await expect(
      page.getByRole("link", { name: /All releases and downloads on GitHub/ }),
    ).toHaveAttribute("href", "https://github.com/jeduden/mdsmith/releases");
  });
});
