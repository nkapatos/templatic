module.exports = async (h) => {
  const a = await h.page(); const b = await h.page({ tag: "js2" });
  await h.open(a, "/"); await h.open(b, "/");
  const q = (p) => new URL(p.url()).search;
  const mark = (p) => p.evaluate(() => (window.__mark = (window.__mark || 0) + 1));
  const marked = (p) => p.evaluate(() => window.__mark);
  await h.check("JS: a tab link swaps the region and pushes the URL, no reload", async () => {
    await mark(a);
    await a.locator(".tabs a", { hasText: "Done" }).click();
    await h.until(async () => q(a).includes("tab=done"), "url tab=done");
    await h.until(async () => (await a.locator('.tabs a[aria-current="page"]').textContent()).trim() === "Done", "region swapped");
    h.eq(await marked(a), 1, "no reload");
  });
  await h.check("JS: opening an item shows it and pushes the URL", async () => {
    const first = a.locator(".list a").first(); const title = (await first.textContent()).trim();
    await first.click();
    await h.until(async () => q(a).includes("open="), "url open=");
    await h.until(async () => (await a.locator(".detail h2").textContent()).trim() === title, "detail");
  });
  await h.check("JS: the other window is untouched", async () => {
    h.eq(q(b), "", "b url");
    h.eq((await b.locator('.tabs a[aria-current="page"]').textContent()).trim(), "Open", "b tab");
  });
  await h.check("JS: back and forward replay the views without a reload", async () => {
    await a.goBack();
    await h.until(async () => !q(a).includes("open=") && q(a).includes("tab=done"), "back url");
    await h.until(async () => (await a.locator(".detail h2").count()) === 0, "detail closed");
    await a.goForward();
    await h.until(async () => q(a).includes("open="), "forward url");
    await h.until(async () => (await a.locator(".detail h2").count()) === 1, "detail back");
    h.eq(await marked(a), 1, "no reload");
  });
  await h.check("JS: typing searches after a pause; refining replaces the entry", async () => {
    await a.locator(".tabs a", { hasText: "Open" }).click();
    await h.until(async () => (await a.locator('.tabs a[aria-current="page"]').textContent()).trim() === "Open" && !q(a).includes("tab=done"), "open tab");
    const len0 = await a.evaluate(() => history.length);
    await a.locator("input[name=q]").pressSequentially("a", { delay: 20 });
    await h.until(async () => q(a).includes("q=a"), "q=a");
    const len1 = await a.evaluate(() => history.length);
    await a.locator("input[name=q]").pressSequentially("b", { delay: 20 });
    await h.until(async () => q(a).includes("q=ab"), "q=ab");
    const len2 = await a.evaluate(() => history.length);
    h.eq([len1 - len0, len2 - len1], [1, 0], "history steps");
    h.eq(await marked(a), 1, "no reload");
  });
  await h.check("JS: back restores the search field from the URL", async () => {
    await a.goBack();
    await h.until(async () => !q(a).includes("q="), "url without q");
    await h.until(async () => (await a.locator("input[name=q]").inputValue()) === "", "field cleared");
  });
  const n = await h.page({ js: false });
  await h.check("no JS: links load the page at their href", async () => {
    await n.goto(h.base + "/");
    await n.locator(".tabs a", { hasText: "Done" }).click(); await n.waitForLoadState();
    h.ok(q(n).includes("tab=done"), "url");
    h.eq((await n.locator('.tabs a[aria-current="page"]').textContent()).trim(), "Done", "rendered");
  });
  await h.check("no JS: the search form GETs the page", async () => {
    await n.locator("input[name=q]").fill("zzzz-nothing");
    await n.locator("form.search button").click(); await n.waitForLoadState();
    h.ok(q(n).includes("q=zzzz-nothing"), "url");
    h.ok((await n.locator(".list").textContent()).includes("Nothing matches"), "empty list");
  });
};
