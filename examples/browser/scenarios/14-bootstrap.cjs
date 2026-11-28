module.exports = async (h) => {
  const a = await h.page(); const b = await h.page({ tag: "js2" });
  await h.open(a, "/"); await h.open(b, "/");
  h.allow(a, (r) => r.request().method() === "POST" && r.status() === 400);
  const card = (p) => p.locator(".card[id^=svc-]").first();
  await h.check("JS: a status button swaps the board region on both tabs", async () => {
    const id = await card(a).getAttribute("id");
    const btn = card(a).locator("form button[name=status]:not(.active)").first(); const st = await btn.getAttribute("value");
    await btn.click();
    await h.until(async () => (await b.locator(`#${id} .card-header .badge`).first().textContent()).trim() === st, "b badge " + st);
  });
  await h.check("JS: a dropdown in a card still works after a redraw (data API on morphed markup)", async () => {
    const t = card(a).locator(".dropdown-toggle"); await t.click();
    await h.until(async () => (await card(a).locator(".dropdown-menu").getAttribute("class")).includes("show"), "dropdown shows");
    await a.keyboard.press("Escape");
  });
  await h.check("JS: the tooltip hook shows on hover, also on a swapped badge", async () => {
    await card(a).locator(".card-header .badge").first().hover();
    await h.until(async () => (await a.locator(".tooltip").count()) > 0, "tooltip");
    await a.mouse.move(0, 0);
  });
  await h.check("JS: the modal form refuses with field errors in the modal, then reports and closes", async () => {
    await a.getByRole("link", { name: "Report incident" }).click();
    const m = a.locator("#report-modal"); await h.until(async () => (await m.getAttribute("class")).includes("show"), "modal open");
    await m.locator("button[type=submit]").click();
    await h.until(async () => (await m.locator("[aria-invalid=true]").count()) > 0, "fields marked");
    await m.locator("select[name=service]").selectOption({ index: 1 });
    await m.locator("input[name=note]").fill("db is slow");
    await m.locator("button[type=submit]").click();
    await h.until(async () => !(await m.getAttribute("class")).includes("show"), "modal closed");
    await h.until(async () => (await b.locator(".badge", { hasText: "incident" }).count()) > 0, "incident on b");
    h.eq(new URL(a.url()).pathname, "/", "stays");
    await h.until(async () => (await a.locator(".modal-backdrop").count()) === 0, "no leftover backdrop");
    h.eq(await a.evaluate(() => document.body.classList.contains("modal-open")), false, "body unlocked");
  });
  const n = await h.page({ js: false });
  h.allow(n, (r) => r.status() === 422 || r.status() === 400);
  await h.check("no JS: status buttons post; the report page refuses then reports", async () => {
    await n.goto(h.base + "/");
    const btn = card(n).locator("form button[name=status]:not(.active)").first(); const st = await btn.getAttribute("value");
    await btn.click(); await n.waitForLoadState();
    h.eq((await card(n).locator(".card-header .badge").first().textContent()).trim(), st, "status rendered");
    await n.goto(h.base + "/report"); await n.locator("form button[type=submit]").click(); await n.waitForLoadState();
    h.ok((await n.locator("[aria-invalid=true]").count()) > 0, "fields marked without JS");
  });
};
