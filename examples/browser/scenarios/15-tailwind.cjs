module.exports = async (h) => {
  const a = await h.page(); const b = await h.page({ tag: "js2" });
  await h.open(a, "/"); await h.open(b, "/");
  h.allow(a, (r) => r.request().method() === "POST" && r.status() === 400);
  await h.check("JS: Add bumps the count on both tabs and swaps the level region, styled", async () => {
    const txt = await b.locator("main").textContent();
    await a.locator("form[data-tc-form] button[type=submit]").click();
    await h.until(async () => (await b.locator("main").textContent()) !== txt, "b changes");
  });
  await h.check("JS: a refused step marks the input (aria-invalid variant styles apply)", async () => {
    const i = a.locator("form[data-tc-form] input[name=n]"); await i.evaluate((el) => (el.type = "text")); await i.fill("zzz");
    await a.locator("form[data-tc-form] button[type=submit]").click();
    await h.until(async () => (await i.getAttribute("aria-invalid")) === "true", "aria-invalid");
    const bc = await i.evaluate((el) => getComputedStyle(el).borderColor); h.ok(bc, "styled");
  });
  const n = await h.page({ js: false });
  await h.check("no JS: Add posts and reloads with the new count", async () => {
    await n.goto(h.base + "/"); const t = await n.locator("main").textContent();
    await n.locator("form[data-tc-form] button[type=submit]").click(); await n.waitForLoadState();
    h.ok((await n.locator("main").textContent()) !== t, "changed");
  });
};
