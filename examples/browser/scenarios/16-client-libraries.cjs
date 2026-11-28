module.exports = async (h) => {
  const a = await h.page(); const b = await h.page({ tag: "js2" });
  await h.open(a, "/"); await h.open(b, "/");
  const order = (p) => p.locator("ol.queue > li").evaluateAll((ls) => ls.map((l) => l.dataset.id));
  await h.check("JS: a drag reorders through the command; the other tab follows", async () => {
    const o0 = await order(a);
    const items = a.locator("ol.queue > li .grip");
    const from = await items.nth(0).boundingBox(), to = await items.nth(2).boundingBox();
    await a.mouse.move(from.x + 3, from.y + 3); await a.mouse.down();
    await a.mouse.move(to.x + 3, to.y + 12, { steps: 12 }); await a.mouse.up();
    await h.until(async () => JSON.stringify(await order(b)) !== JSON.stringify(o0), "b reordered");
    await h.until(async () => JSON.stringify(await order(a)) === JSON.stringify(await order(b)), "a settles to the server's order");
  });
  await h.check("JS: an arrow button moves a task", async () => {
    const o0 = await order(a);
    await a.locator("ol.queue > li").nth(1).getByRole("button", { name: /up$/ }).click();
    await h.until(async () => (await order(b))[0] === o0[1], "b moved up");
  });
  await h.check("JS: intl hook formats numbers and dates in the browser", async () => {
    const d = a.locator("dl.stats data").first();
    await h.until(async () => (await d.textContent()) !== (await d.getAttribute("value")), "formatted");
  });
  await h.check("JS: the map island loads Leaflet only on /map and draws markers", async () => {
    const urls = []; a.on("request", (r) => urls.push(r.url()));
    h.ok(!urls.some((u) => /place-map/.test(u)), "not on /");
    await a.getByRole("link", { name: "Map" }).click(); await a.waitForURL(/map/); await h.ready(a);
    await h.until(async () => (await a.locator(".leaflet-container").count()) === 1, "leaflet container");
    await h.until(async () => (await a.locator(".leaflet-interactive").count()) > 0, "markers");
  });
  const n = await h.page({ js: false });
  await h.check("no JS: arrows post and redirect; the map page shows its table", async () => {
    await n.goto(h.base + "/"); const o0 = await order(n);
    await n.locator("ol.queue > li").nth(1).getByRole("button", { name: /up$/ }).click(); await n.waitForLoadState();
    h.eq((await order(n))[0], o0[1], "moved");
    await n.goto(h.base + "/map"); h.ok(await n.locator("table.fallback").isVisible(), "fallback table");
  });
};
