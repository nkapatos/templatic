module.exports = async (h) => {
  const a = await h.page(); const b = await h.page({ tag: "js2" });
  await h.open(a, "/"); await h.open(b, "/?page=2");
  const q = (p) => new URL(p.url()).searchParams;
  const firstId = (p) => p.locator("tbody tr td.col-id").first().textContent();
  await h.check("JS: sorting by a column swaps the table, pushes the URL; again flips direction", async () => {
    await a.evaluate(() => (window.__mark = 1));
    await a.locator("thead th.col-amount a").click();
    await h.until(() => q(a).get("sort") === "amount", "sort=amount");
    const dir1 = q(a).get("dir");
    await h.until(async () => (await a.locator("th.col-amount").getAttribute("aria-sort")) !== "none", "aria-sort");
    await a.locator("thead th.col-amount a").click();
    await h.until(() => q(a).get("dir") !== dir1, "dir flipped");
    h.eq(await a.evaluate(() => window.__mark), 1, "no reload");
  });
  await h.check("JS: pager next moves to page 2", async () => {
    await a.locator('.pager a[rel="next"]').click();
    await h.until(() => q(a).get("page") === "2", "page=2");
  });
  await h.check("JS: an update refreshes each tab with its own view", async () => {
    const rowsCell = a.locator("p.muted strong").first();
    const lastA = await a.locator("p.muted strong").nth(1).textContent();
    await h.until(async () => (await a.locator("p.muted strong").nth(1).textContent()) !== lastA, "last update cell ticks", 5000);
    h.eq(q(b).get("page"), "2", "b still on page 2");
    h.ok((await b.locator('.pager a[rel="prev"]').count()) === 1, "b shows page 2's pager");
    h.eq(q(a).get("sort"), "amount", "a keeps its sort");
    h.ok(await rowsCell.textContent(), "rows cell");
  });
  await h.check("JS: typing in search survives a refresh and filters", async () => {
    const box = a.locator("form.search input[type=search], form.search input[name=q]").first();
    await box.click(); await box.pressSequentially("pa", { delay: 30 });
    await h.until(() => q(a).get("q") === "pa", "q=pa");
    const lastA = await a.locator("p.muted strong").nth(1).textContent();
    await h.until(async () => (await a.locator("p.muted strong").nth(1).textContent()) !== lastA, "a refresh", 5000);
    h.eq(await box.inputValue(), "pa", "typed text kept");
    h.ok(await box.evaluate((el) => document.activeElement === el), "focus kept");
  });
  await h.check("JS: ?page=999 shows the last page", async () => {
    await h.open(b, "/?page=999");
    h.eq(await b.locator('.pager a[rel="next"]').count(), 0, "no next");
  });
  const n = await h.page({ js: false });
  await h.check("no JS: header links and search load pages", async () => {
    await n.goto(h.base + "/");
    await n.locator("thead th.col-amount a").click(); await n.waitForLoadState();
    h.eq(q(n).get("sort"), "amount", "sort");
    await n.locator("form.search input[name=q]").fill("zzzz"); await n.locator("form.search button").click(); await n.waitForLoadState();
    h.eq(q(n).get("q"), "zzzz", "q");
    h.ok((await n.locator("tbody").textContent()).includes("No orders match"), "empty");
  });
};
