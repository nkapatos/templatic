module.exports = async (h) => {
  const a = await h.page(); const b = await h.page({ tag: "js2" });
  await h.open(a, "/"); await h.open(b, "/");
  await h.check("JS: like on the detail page updates its count and the list on another tab", async () => {
    const first = a.locator("a[href^='/items/']").first(); const href = await first.getAttribute("href");
    const id = href.split("/").pop();
    const listCell = b.locator(`a[href='${href}']`).locator("xpath=..").locator(".likes");
    const before = parseInt(await listCell.textContent());
    await first.click(); await a.waitForURL(/\/items\//); await h.ready(a);
    await a.getByRole("button", { name: "Like" }).click();
    await h.until(async () => parseInt(await a.locator(".count").textContent()) === before + 1, "detail +1");
    await h.until(async () => parseInt(await listCell.textContent()) === before + 1, "list on b +1 (" + id + ")");
    h.ok(/\/items\//.test(a.url()), "stays");
  });
  const n = await h.page({ js: false });
  await h.check("no JS: like posts and redirects back to the item", async () => {
    await n.goto(h.base + "/"); await n.locator("a[href^='/items/']").first().click(); await n.waitForLoadState();
    const before = parseInt(await n.locator(".count").textContent());
    await n.getByRole("button", { name: "Like" }).click(); await n.waitForLoadState();
    h.ok(/\/items\//.test(n.url()), "back on the item");
    h.eq(parseInt(await n.locator(".count").textContent()), before + 1, "+1");
  });
  await h.check("an unknown item is a 404", async () => {
    h.allow(n, (r) => r.status() === 404);
    const r = await n.goto(h.base + "/items/nope"); h.eq(r.status(), 404, "status");
  });
};
