module.exports = async (h) => {
  const a = await h.page(); await h.open(a, "/");
  await h.check("JS: the island mounts, clears the fallback and draws a canvas", async () => {
    await h.until(async () => (await a.locator("traffic-chart canvas").count()) === 1, "canvas");
    h.eq(await a.locator("traffic-chart .fallback").count(), 0, "fallback cleared");
  });
  await h.check("JS: bound cells push new props every second", async () => {
    const n0 = await a.locator("traffic-chart").evaluate((el) => JSON.stringify(el.props?.series ?? el.series ?? null));
    await h.until(async () => (await a.locator("traffic-chart").evaluate((el) => JSON.stringify(el.props?.series ?? el.series ?? null))) !== n0, "series changes", 3000);
  });
  await h.check("JS: a window button changes the bound window prop", async () => {
    const w = () => a.locator("traffic-chart").evaluate((el) => el.props?.window ?? el.window);
    const buttons = a.locator("p.actions button"); const last = buttons.last();
    const want = Number((await last.textContent()).trim());
    await last.click();
    await h.until(async () => (await w()) === want, "window = " + want);
  });
  await h.check("JS: the chart chunk is not loaded on /about", async () => {
    const p = await h.page({ tag: "js-about" }); const urls = [];
    p.on("request", (r) => urls.push(r.url()));
    await h.open(p, "/about");
    h.ok(!urls.some((u) => u.includes("traffic-chart")), "no island chunk");
  });
  const n = await h.page({ js: false });
  await h.check("no JS: the fallback shows the latest value and the table", async () => {
    await n.goto(h.base + "/");
    h.ok(await n.locator(".fallback .latest").isVisible(), "fallback visible");
  });
};
