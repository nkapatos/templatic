module.exports = async (h) => {
  const a = await h.page(); const statuses = {};
  a.on("response", (r) => (statuses[new URL(r.url()).pathname] = { s: r.status(), cc: r.headers()["cache-control"] || "" }));
  h.allow(a, (r) => r.status() === 404 && r.url().endsWith("/nope"));
  await h.check("JS: pages load, CSS and the logo it references come from the bundle, hashed and cached", async () => {
    await a.goto(h.base + "/"); await a.waitForLoadState("networkidle");
    const css = Object.keys(statuses).find((p) => /\/_templatic\/.*\.css$/.test(p)); h.ok(css, "css loaded");
    h.ok(/immutable/.test(statuses[css].cc), "css immutable: " + statuses[css].cc);
    h.ok(Object.keys(statuses).some((p) => /\/_templatic\/.*\.svg$/.test(p)), "bundled image");
    h.eq(await a.evaluate(() => tcState().live.connection === "open"), false, "no live stream");
    await a.getByRole("link", { name: /about/i }).first().click(); await a.waitForURL(/about/);
    const r = await a.goto(h.base + "/nope"); h.eq(r.status(), 404, "404 page");
  });
};
