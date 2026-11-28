module.exports = async (h) => {
  const a = await h.page(); const b = await h.page({ tag: "js2" });
  const enc = {}; a.on("response", (r) => { const p = new URL(r.url()).pathname; enc[p] = r.headers()["content-encoding"] || ""; });
  await h.open(a, "/"); await h.open(b, "/");
  h.allow(a, (r) => r.request().method() === "POST" && r.status() === 400);
  const count = (p) => p.locator(".count").textContent();
  await h.check("JS: Add updates both tabs; a bad n marks the field", async () => {
    const c0 = Number(await count(b));
    const f = a.locator("form[data-tc-form]");
    await f.locator("input[name=n]").fill("3"); await f.locator("button").click();
    await h.until(async () => Number(await count(b)) === c0 + 3, "b +3");
    await f.locator("input[name=n]").fill("abc").catch(() => {}); await f.locator("input[name=n]").evaluate((el) => (el.type = "text"));
    await f.locator("input[name=n]").fill("abc"); await f.locator("button").click();
    await h.until(async () => ((await f.locator('[data-tc-field-error="n"]').textContent()) || "") !== "", "n slot");
  });
  await h.check("HTML and JS are gzipped, the event stream is not", async () => {
    const page = Object.keys(enc).find((p) => p === "/"); h.eq(enc[page], "gzip", "page");
    const js = Object.keys(enc).find((p) => p.endsWith(".js")); h.eq(enc[js], "gzip", "js " + js);
    h.eq(enc["/live/stream"], "", "stream");
  });
  const n = await h.page({ js: false });
  h.allow(n, (r) => r.status() === 422);
  await h.check("no JS: Add posts and redirects; the JS tab follows", async () => {
    await n.goto(h.base + "/"); const c0 = Number(await count(a));
    await n.locator("form[data-tc-form] button").click(); await n.waitForLoadState();
    await h.until(async () => Number(await count(a)) === c0 + 1, "a +1");
  });
};
