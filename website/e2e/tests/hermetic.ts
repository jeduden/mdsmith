import {
  test as base,
  expect,
  type BrowserContext,
} from "@playwright/test";

/**
 * Hermetic-network test fixtures.
 *
 * The homepage hero embeds four third-party badge images (github.com,
 * goreportcard.com, codecov.io, img.shields.io) marked loading="lazy".
 * With JavaScript enabled, lazy images never gate the window load
 * event. With JavaScript disabled, Chromium loads lazy images eagerly
 * (the HTML spec's lazy-load steps require scripting), so page.goto's
 * default waitUntil: "load" blocks on all four external hosts — and a
 * hanging badge host times out the no-JS tests after 30s, which is
 * exactly the failure CI hit while every JS-enabled test stayed green.
 *
 * blockCrossOrigin aborts any request that leaves the local test
 * server, at the network-routing layer (independent of page JS), so an
 * external <img> fails immediately and load fires. The suite then
 * exercises only the Hugo output it built, regardless of third-party
 * host availability.
 */
export async function blockCrossOrigin(ctx: BrowserContext): Promise<void> {
  await ctx.route("**/*", (route) => {
    const host = new URL(route.request().url()).hostname;
    if (host === "localhost" || host === "127.0.0.1") {
      return route.continue();
    }
    return route.abort("blockedbyclient");
  });
}

/** Drop-in replacement for @playwright/test's `test` whose default
 * context blocks cross-origin requests. Tests that create their own
 * context (e.g. to disable JavaScript) must call blockCrossOrigin on
 * it themselves. */
export const test = base.extend({
  context: async ({ context }, use) => {
    await blockCrossOrigin(context);
    await use(context);
  },
});

export { expect };
