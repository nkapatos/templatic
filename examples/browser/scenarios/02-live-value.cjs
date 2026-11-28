module.exports = async (h) => {
  const level = async (p) => Number(await p.locator('[data-tc-cell="tank.level"]').textContent());
  const meter = async (p) => Number(await p.locator("meter").getAttribute("value"));
  const a = await h.page(); const b = await h.page({ tag: "js2" });
  await h.open(a, "/"); await h.open(b, "/");
  await h.check("JS: the server's drain updates the cell and the meter attribute live", async () => {
    const v = await level(a);
    await h.until(async () => (await level(a)) !== v, "drain tick", 3000);
    await h.until(async () => (await meter(a)) === (await level(a)), "meter follows");
  });
  await h.check("JS: Fill 50 raises the level on both tabs, page stays", async () => {
    await a.evaluate(() => (window.__mark = 1));
    const v = await level(b);
    await a.getByRole("button", { name: "Fill 50" }).click();
    await h.until(async () => (await level(b)) >= Math.min(100, v + 40), "tab b sees fill");
    h.eq(await a.evaluate(() => window.__mark), 1, "no reload");
  });
  await h.check("JS: Empty drops it to 0", async () => {
    await a.getByRole("button", { name: "Empty" }).click();
    await h.until(async () => (await level(b)) === 0, "tab b sees empty");
  });
  const n = await h.page({ js: false });
  await h.check("no JS: Fill 50 posts, comes back to the page with the new level", async () => {
    await n.goto(h.base + "/");
    await n.getByRole("button", { name: "Fill 50" }).click(); await n.waitForLoadState();
    h.eq(new URL(n.url()).pathname, "/", "back on the page");
    h.ok((await level(n)) >= 45, "level rendered");
    await h.until(async () => (await level(a)) >= 45, "JS tab follows");
  });
};
