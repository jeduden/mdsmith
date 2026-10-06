import { test, expect, blockCrossOrigin } from "./hermetic";

/**
 * Homepage tests for the forge layout.
 *
 * The hero carries the headline, lead, calls to action, the
 * markdownlint migration link, and the live badges. Beside it a card
 * stack shows real Markdown sources behind an animated terminal that
 * replays `mdsmith check` -> `fix` -> `check`. Below, "Why mdsmith"
 * and one block per feature group each sit on a dimmed copy of their
 * own source section in docs/features/index.md. The tests also guard
 * the feature rows (no MDS rule-ID codes in row copy) and the install
 * rows on a narrow viewport.
 */
test.describe("homepage hero", () => {
  test("hero lead names the product category", async ({ page }) => {
    await page.goto("/");

    const lead = page.locator(".fx-lead");
    await expect(lead).toBeVisible();
    await expect(lead).toContainText("Markdown linter and formatter");
    // The lead's job is category + promise; the concrete scope
    // ("cross-file integrity", auto-fix, …) belongs to the
    // "Why mdsmith" lead below. Guard against the copy drifting
    // back into a feature enumeration that duplicates it.
    await expect(lead).not.toContainText("cross-file integrity");
  });

  test("hero links markdownlint users to the migration guide", async ({
    page,
  }) => {
    await page.goto("/");

    const link = page.locator(".fx-switch a");
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

    const badges = page.locator(".fx-badges a");
    const count = await badges.count();
    expect(count).toBeGreaterThan(0);
    for (let i = 0; i < count; i++) {
      await expect(badges.nth(i)).toBeHidden();
    }
  });

  test("the eyebrow, positioning band, and channel strip are gone", async ({
    page,
  }) => {
    await page.goto("/");

    // The eyebrow still names the page in <title>, but the hero no
    // longer prints it (the phrase stays elsewhere on the page as a
    // feature-group name), and the scope-statement band and the
    // "Available on" channel strip are gone.
    await expect(page).toHaveTitle(/Markdown as a single source of truth/);
    await expect(page.locator(".fx-hero")).not.toContainText(
      "Markdown as a single source of truth",
    );
    await expect(page.locator("main")).not.toContainText("Available on");
    await expect(page.locator(".positioning")).toHaveCount(0);
    await expect(page.locator(".logos")).toHaveCount(0);
  });

  test("the home page uses the inverse logo lockup", async ({ page }) => {
    await page.goto("/");

    for (const where of [".topnav", ".footer"]) {
      await expect(page.locator(`${where} .topnav-brand img`)).toHaveAttribute(
        "src",
        /logo-lockup-inverse\.svg$/,
      );
    }
  });
});

test.describe("homepage forge stack", () => {
  test("without motion the terminal shows the finished run", async ({
    page,
  }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto("/");

    const stack = page.locator(".fx-stack");
    await expect(stack).toBeVisible();
    const screen = stack.locator(".fx-term-body");
    await expect(screen).toHaveAttribute("role", "img");
    await expect(screen).toHaveAttribute("aria-label", /failures=0/);
    // innerText: only lines that are actually shown count.
    const shown = { useInnerText: true };
    await expect(screen).toContainText("MDS006", shown);
    await expect(screen).toContainText("MDS012", shown);
    await expect(screen).toContainText("failures=0", shown);
    // The intro.md card behind the terminal shows the fixed file.
    await expect(stack).toHaveClass(/is-fixed/);
    await expect(stack).not.toHaveAttribute("data-playing", "true");
    await expect(stack.locator(".fx-term-toggle")).toBeHidden();
  });

  test("the terminal replays check, fix, check", async ({ page }) => {
    await page.goto("/");

    const stack = page.locator(".fx-stack");
    await expect(stack).toHaveAttribute("data-playing", "true");
    const screen = stack.locator(".fx-term-body");
    const shown = { useInnerText: true, timeout: 15_000 };
    // The animation restarts from an empty terminal, finds the two
    // diagnostics, then marks the intro.md card fixed once the
    // `mdsmith fix` step lands.
    await expect(stack).not.toHaveClass(/is-fixed/);
    await expect(screen).not.toContainText("MDS006", { useInnerText: true });
    await expect(screen).toContainText("MDS006", shown);
    await expect(stack).toHaveClass(/is-fixed/, { timeout: 15_000 });
    await expect(screen).toContainText("failures=0", shown);
  });

  test("the pause control stops and restarts the replay", async ({ page }) => {
    await page.goto("/");

    const stack = page.locator(".fx-stack");
    const toggle = stack.locator(".fx-term-toggle");
    await expect(toggle).toBeVisible();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await expect(stack).toHaveAttribute("data-playing", "false");

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
    await expect(stack).toHaveAttribute("data-playing", "true");
  });

  test("with JavaScript off the full transcript is still readable", async ({
    browser,
  }) => {
    const context = await browser.newContext({ javaScriptEnabled: false });
    await blockCrossOrigin(context);
    const page = await context.newPage();
    await page.goto("/");

    const screen = page.locator(".fx-stack .fx-term-body");
    await expect(screen).toContainText("MDS006", { useInnerText: true });
    await expect(screen).toContainText("failures=0", { useInnerText: true });
    await context.close();
  });

  test("background cards show Markdown, not front matter", async ({
    page,
  }) => {
    await page.goto("/");

    const cards = page.locator(".fx-stack .fx-card");
    expect(await cards.count()).toBeGreaterThanOrEqual(3);
    const text = (await cards.allTextContents()).join("\n");
    expect(text).toContain("# Quickstart");
    expect(text).toContain("# Why mdsmith");
    expect(text).not.toMatch(/^\s*(title|summary|weight|group):/m);
  });
});

test.describe("homepage feature groups", () => {
  test("each feature group sits on its own source section", async ({
    page,
  }) => {
    await page.goto("/");

    const why = page.locator(".fx-why .fx-src");
    await expect(why).toHaveAttribute("aria-hidden", "true");
    await expect(why).toContainText("# Why mdsmith");

    const pillars = page.locator(".fx-pillar");
    await expect(pillars).toHaveCount(5);
    for (let i = 0; i < 5; i++) {
      const pillar = pillars.nth(i);
      const name = (await pillar.locator(".fx-pillar-name").innerText()).trim();
      const src = pillar.locator(".fx-src");
      await expect(src).toHaveAttribute("aria-hidden", "true");
      // The source layer is that group's `## <name>` section of
      // docs/features/index.md, so the page and its source agree.
      await expect(src).toContainText(`## ${name}`);
    }
  });

  test("every feature page has a row that links to it", async ({ page }) => {
    await page.goto("/");

    const rows = page.locator(".fx-pillar a.fx-row");
    expect(await rows.count()).toBeGreaterThanOrEqual(19);
    await expect(rows.first()).toHaveAttribute("href", /\/features\/[a-z-]+\/$/);
    await expect(rows.first().locator(".fx-row-id")).toHaveText("01.1");
  });

  test("feature rows carry no opaque MDS rule-ID codes", async ({ page }) => {
    await page.goto("/");

    // The codes are defined on the Rules index and cited on each
    // feature page; on the homepage they are undecodable to a
    // first-time visitor, so the rows omit them. The aria-hidden
    // source layers behind the rows quote docs/features/index.md
    // verbatim and are not part of the row copy.
    const copy = await page.locator("a.fx-row").allInnerTexts();
    expect(copy.length).toBeGreaterThan(5);
    for (const text of copy) {
      expect(text).not.toMatch(/\bMDS\d{3}\b/);
    }
    const attrs = await page.locator("a.fx-row").evaluateAll(rows =>
      rows.flatMap(row =>
        [row, ...Array.from(row.querySelectorAll("*"))].flatMap(el =>
          Array.from(el.attributes).map(a => a.value),
        ),
      ),
    );
    for (const value of attrs) {
      expect(value).not.toMatch(/\bMDS\d{3}\b/);
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

  test("the page does not scroll sideways on a phone", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/");

    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth,
    );
    expect(overflow).toBeLessThanOrEqual(0);
  });
});
