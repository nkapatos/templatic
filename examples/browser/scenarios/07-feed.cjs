module.exports = async (h) => {
  const a = await h.page(); const b = await h.page({ tag: "js2" }); const r = await h.page({ tag: "js-random" });
  await h.open(a, "/rooms/general"); await h.open(b, "/rooms/general"); await h.open(r, "/rooms/random");
  h.allow(a, (x) => x.request().method() === "POST" && x.status() === 400);
  const msgs = (p) => p.locator("section ol > li");
  const post = a.locator("form.post");
  await h.check("JS: posting adds the message at the bottom on both general tabs, not on random; field cleared", async () => {
    const nr = await msgs(r).count();
    await post.locator("input[name=name]").fill("ann"); await post.locator("input[name=text]").fill("hello-1");
    await post.locator("button").click();
    await h.until(async () => (await msgs(b).last().textContent()).includes("hello-1"), "b got it at the bottom");
    await h.until(async () => (await msgs(a).last().textContent()).includes("hello-1"), "a got it");
    await h.until(async () => (await post.locator("input[name=text]").inputValue()) === "", "cleared");
    await a.waitForTimeout(300); h.eq(await msgs(r).count(), nr, "random untouched");
  });
  await h.check("JS: an empty post marks the field", async () => {
    await post.locator("input[name=text]").fill(""); await post.locator("button").click();
    await h.until(async () => ((await post.locator('[data-tc-field-error="text"]').textContent()) || "") !== "", "text slot");
  });
  await h.check("JS: like on a live-inserted message counts on both tabs; a double click counts once", async () => {
    const li = msgs(a).filter({ hasText: "hello-1" });
    const btn = li.locator('button[aria-label="Like"]');
    await btn.evaluate((el) => { el.form.requestSubmit(el); el.form.requestSubmit(el); });
    await h.until(async () => (await msgs(b).filter({ hasText: "hello-1" }).locator('button[aria-label="Like"]').textContent()).includes("1"), "b sees 1 like");
    await a.waitForTimeout(400);
    h.ok((await btn.textContent()).includes("♥ 1"), "counted once: " + (await btn.textContent()));
    await btn.click();
    await h.until(async () => (await msgs(b).filter({ hasText: "hello-1" }).locator('button[aria-label="Like"]').textContent()).includes("2"), "second like");
  });
  await h.check("JS: typing keeps value and focus while a like re-renders a message", async () => {
    const box = post.locator("input[name=text]"); await box.click(); await box.pressSequentially("typing", { delay: 10 });
    await msgs(b).filter({ hasText: "hello-1" }).locator('button[aria-label="Like"]').click();
    await h.until(async () => (await msgs(a).filter({ hasText: "hello-1" }).locator('button[aria-label="Like"]').textContent()).includes("3"), "like arrives");
    h.eq(await box.inputValue(), "typing", "value kept");
    h.ok(await box.evaluate((el) => document.activeElement === el), "focus kept");
    await box.fill("");
  });
  await h.check("JS: delete removes it everywhere", async () => {
    await msgs(a).filter({ hasText: "hello-1" }).locator('button[aria-label="Delete"]').click();
    await h.until(async () => (await msgs(b).filter({ hasText: "hello-1" }).count()) === 0, "gone on b");
  });
  await h.check("JS: the activity log gets new lines on top", async () => {
    const log = a.locator("aside ol > li, aside ul > li");
    const top = await log.first().textContent();
    const f = b.locator("form.post");
    await f.locator("input[name=name]").fill("cy"); await f.locator("input[name=text]").fill("for-the-log");
    await f.locator("button").click();
    await h.until(async () => (await log.first().textContent()) !== top, "new top line");
    h.ok((await log.first().getAttribute("class") || "").includes("post"), "post class on the new line");
  });
  const n = await h.page({ js: false });
  h.allow(n, (x) => x.status() === 422 || x.status() === 400);
  await h.check("no JS: post redirects back with the name kept; JS tabs get it live", async () => {
    await n.goto(h.base + "/rooms/general");
    const f = n.locator("form.post");
    await f.locator("input[name=name]").fill("bob"); await f.locator("input[name=text]").fill("plain-1");
    await f.locator("button").click(); await n.waitForLoadState();
    h.ok((await msgs(n).last().textContent()).includes("plain-1"), "rendered");
    h.eq(await n.locator("form.post input[name=name]").inputValue(), "bob", "name kept");
    await h.until(async () => (await msgs(a).last().textContent()).includes("plain-1"), "a live");
  });
  await h.check("no JS: like and delete post and come back", async () => {
    const li = msgs(n).filter({ hasText: "plain-1" });
    await li.locator('button[aria-label="Like"]').click(); await n.waitForLoadState();
    h.ok((await msgs(n).filter({ hasText: "plain-1" }).locator('button[aria-label="Like"]').textContent()).includes("1"), "liked");
    await msgs(n).filter({ hasText: "plain-1" }).locator('button[aria-label="Delete"]').click(); await n.waitForLoadState();
    h.eq(await msgs(n).filter({ hasText: "plain-1" }).count(), 0, "deleted");
    h.ok(/\/rooms\/general/.test(n.url()), "back in the room");
  });
};
