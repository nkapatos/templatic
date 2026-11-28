module.exports = async (h) => {
  const a = await h.page(); const b = await h.page({ tag: "js2" });
  await h.open(a, "/"); await h.open(b, "/");
  h.allow(a, (r) => r.request().method() === "POST" && r.status() === 400);
  const tab = (p, v) => p.locator(`#tab-${v}`);
  await h.check("JS: a redraw resets b's tabs to Open (documented), and the tabs still work", async () => {
    await tab(b, "done").click();
    await h.until(async () => (await b.locator("#panel-done").isVisible()), "done panel on b");
    await a.locator("form[data-tc-form] input[name=title]").fill("from-a");
    await a.locator("form[data-tc-form] button[type=submit]").first().click();
    await h.until(async () => (await b.locator("#panel-open").textContent()).includes("from-a"), "b redrawn");
    await h.until(async () => (await b.locator("#panel-open").isVisible()), "b back on Open");
    await tab(b, "done").click();
    await h.until(async () => (await b.locator("#panel-done").isVisible()), "Done works after the redraw");
    await tab(b, "done").focus(); await b.keyboard.press("ArrowLeft");
    await h.until(async () => (await b.evaluate(() => document.activeElement?.id)) === "tab-open", "arrow-key focus after the redraw");
  });
  await h.check("JS: an empty title marks the field", async () => {
    await a.locator("form[data-tc-form] input[name=title]").fill("");
    await a.locator("form[data-tc-form] button[type=submit]").first().click();
    await h.until(async () => (await a.locator("input[name=title]").getAttribute("aria-invalid")) === "true", "marked");
  });
  await h.check("JS: Done moves a task to the done tab on both pages", async () => {
    const li = a.locator("#panel-open li", { hasText: "from-a" });
    await li.getByRole("button", { name: "Done" }).click();
    await h.until(async () => (await b.locator("#panel-done").textContent()).includes("from-a"), "done on b");
  });
  await h.check("JS: the about dialog opens and closes, outside the region", async () => {
    await a.getByRole("button", { name: /about/i }).first().click();
    await h.until(async () => (await a.locator("[role=dialog], dialog").first().isVisible()), "open");
    await a.keyboard.press("Escape");
    await h.until(async () => !(await a.locator("[role=dialog], dialog").first().isVisible()), "closed");
  });
  const n = await h.page({ js: false });
  h.allow(n, (r) => r.status() === 422);
  await h.check("no JS: add posts and redirects", async () => {
    await n.goto(h.base + "/"); await n.locator("input[name=title]").fill("plain-task");
    await n.locator("form[data-tc-form] button[type=submit]").first().click(); await n.waitForLoadState();
    h.ok((await n.locator("main").textContent()).includes("plain-task"), "listed");
  });
};
