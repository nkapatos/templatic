module.exports = async (h) => {
  const a = await h.page(); await h.open(a, "/");
  await h.check("JS: menu closes on Escape and on an outside click", async () => {
    const m = a.locator("header details.menu, details.menu").first();
    await m.locator("summary").click(); h.ok(await m.evaluate((d) => d.open), "opened");
    await a.keyboard.press("Escape"); await h.until(() => m.evaluate((d) => !d.open), "escape closes");
    await m.locator("summary").click(); await a.mouse.click(5, 5); await h.until(() => m.evaluate((d) => !d.open), "outside click closes");
  });
  await h.check("JS: theme toggle flips data-theme without a reload, cookie survives a reload", async () => {
    await a.evaluate(() => (window.__mark = 1));
    const t0 = await a.locator("html").getAttribute("data-theme");
    await a.locator("form.theme button").click();
    await h.until(async () => (await a.locator("html").getAttribute("data-theme")) !== t0, "flipped");
    h.eq(await a.evaluate(() => window.__mark), 1, "no reload");
    const t1 = await a.locator("html").getAttribute("data-theme");
    await a.reload(); await h.ready(a);
    h.eq(await a.locator("html").getAttribute("data-theme"), t1, "server paints the cookie");
  });
  await h.check("JS: help opens as a modal; Escape and backdrop close it", async () => {
    await a.getByRole("link", { name: "Open help" }).click();
    await h.until(() => a.locator("#help-dialog").evaluate((d) => d.open), "dialog open");
    h.eq(new URL(a.url()).pathname, "/", "stays on page");
    await a.keyboard.press("Escape"); await h.until(() => a.locator("#help-dialog").evaluate((d) => !d.open), "escape");
    await a.getByRole("link", { name: "Open help" }).click();
    await h.until(() => a.locator("#help-dialog").evaluate((d) => d.open), "dialog open again");
    await a.mouse.click(3, 3); await h.until(() => a.locator("#help-dialog").evaluate((d) => !d.open), "backdrop");
  });
  await h.check("JS: Next tip swaps the card, moves the URL, and the new card's menu hook works", async () => {
    const id0 = await a.locator("article.tip").getAttribute("id");
    await a.locator("article.tip a", { hasText: /next/i }).last().click();
    await h.until(async () => (await a.locator("article.tip").getAttribute("id")) !== id0, "card swapped");
    h.ok(/tip=/.test(a.url()), "url moved");
    const m = a.locator("article.tip details.menu");
    await m.locator("summary").click(); h.ok(await m.evaluate((d) => d.open), "opened");
    await a.keyboard.press("Escape"); await h.until(() => m.evaluate((d) => !d.open), "hook on swapped card");
  });
  const n = await h.page({ js: false });
  await h.check("no JS: help link opens the page; theme posts and redirects back; next tip loads", async () => {
    await n.goto(h.base + "/");
    await n.getByRole("link", { name: "Open help" }).click(); await n.waitForLoadState();
    h.eq(new URL(n.url()).pathname, "/help", "help page");
    await n.goto(h.base + "/");
    const t0 = await n.locator("html").getAttribute("data-theme");
    await n.locator("form.theme button").click(); await n.waitForLoadState();
    h.ok((await n.locator("html").getAttribute("data-theme")) !== t0, "theme flipped by server");
    await n.locator("article.tip a", { hasText: /next/i }).last().click(); await n.waitForLoadState();
    h.ok(/tip=/.test(n.url()), "tip url");
  });
};
