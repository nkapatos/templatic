module.exports = async (h) => {
  const slot = (p, n) => p.locator(`form [data-tc-field-error="${n}"]`).first().textContent();
  const count = (p) => p.locator(".count").textContent();
  const fill = async (p, f, v) => {
    if (v.title !== undefined) await f.locator("input[name=title]").fill(v.title);
    if (v.pages !== undefined) await f.locator("input[name=pages]").fill(v.pages);
    if (v.shelf !== undefined) await f.locator("select[name=shelf]").selectOption(v.shelf);
    if (v.fav !== undefined) await f.locator("input[name=favorite]").setChecked(v.fav);
  };
  const a = await h.page(); const b = await h.page({ tag: "js2" });
  h.allow(a, (r) => r.request().method() === "POST" && [400, 404].includes(r.status()));
  await h.open(a, "/"); await h.open(b, "/");
  const form = a.locator("form[data-tc-form]");

  await h.check("JS: a refused save marks every field and the form error, stays on the page", async () => {
    await fill(a, form, { title: "", pages: "0", shelf: "", fav: true });
    await form.locator("button[type=submit]").click();
    await h.until(async () => (await slot(a, "title")) === "enter a title", "title slot");
    h.eq(await slot(a, "pages"), "between 1 and 5000", "pages slot");
    h.eq(await form.locator("[data-tc-form-error]").textContent(), "only a finished book can be a favorite", "form error");
    h.eq(await form.locator("input[name=title]").getAttribute("aria-invalid"), "true", "aria-invalid");
    h.eq(new URL(a.url()).pathname, "/", "url");
  });

  await h.check("JS: a good save redraws the list on both tabs, clears marks, resets the form", async () => {
    const before = await count(b);
    await fill(a, form, { title: "Dune", pages: "412", shelf: "reading", fav: false });
    await form.locator("input[name=tags][value=fiction]").check();
    await form.locator("button[type=submit]").click();
    await h.until(async () => (await count(b)) !== before, "list redraw on tab b");
    await h.until(async () => (await count(a)) === (await count(b)), "list redraw on tab a");
    h.eq(await slot(a, "title"), "", "title slot cleared");
    h.eq(await form.locator("input[name=title]").getAttribute("aria-invalid"), null, "aria-invalid cleared");
    await h.until(async () => (await form.locator("input[name=title]").inputValue()) === "", "form reset by hook");
    h.ok((await a.locator(".books").textContent()).includes("Dune"), "Dune listed");
  });

  await h.check("JS: a second submit right after the first saves once", async () => {
    const before = await count(b);
    await fill(a, form, { title: "Twice", pages: "10", shelf: "reading" });
    await form.evaluate((f) => { f.requestSubmit(); f.requestSubmit(); });
    await h.until(async () => (await count(b)) !== before, "redraw");
    await a.waitForTimeout(500);
    h.eq((await a.locator(".books li", { hasText: "Twice" }).count()), 1, "Twice listed once");
  });

  await h.check("JS: the edit page saves in place, path id wins", async () => {
    await a.locator(".books a", { hasText: "Dune" }).click();
    await a.waitForURL(/\/books\//); await h.ready(a);
    const f = a.locator("form[data-tc-form]");
    await fill(a, f, { title: "Dune Messiah" });
    await f.locator("button[type=submit]").click();
    await h.until(async () => (await b.locator(".books").textContent()).includes("Dune Messiah"), "renamed on tab b");
    h.ok(/\/books\//.test(a.url()), "stays on the edit page");
  });

  await h.check("JS: saving an unknown book shows the 404 message in the form error", async () => {
    const f = a.locator("form[data-tc-form]");
    await f.evaluate((el) => el.setAttribute("action", "/books/nope"));
    await f.locator("button[type=submit]").click();
    await h.until(async () => ((await f.locator("[data-tc-form-error]").textContent()) || "").length > 0, "form error filled");
  });

  const n = await h.page({ js: false });
  h.allow(n, (r) => [404, 422].includes(r.status()));
  await h.check("no JS: GET unknown book is a 404 page", async () => {
    const r = await n.goto(h.base + "/books/nope"); h.eq(r.status(), 404, "status");
  });
  await h.check("no JS: a refused save re-renders with 422, errors and what was typed", async () => {
    await n.goto(h.base + "/");
    const f = n.locator("form[data-tc-form]");
    await fill(n, f, { title: "Keep me", pages: "0", shelf: "reading" });
    await f.locator("input[name=tags][value=science]").check();
    const [r] = await Promise.all([n.waitForResponse((r) => r.request().method() === "POST"), f.locator("button[type=submit]").click()]);
    h.eq(r.status(), 422, "status");
    await n.waitForLoadState();
    h.eq(await slot(n, "pages"), "between 1 and 5000", "pages slot");
    h.eq(await n.locator("form input[name=title]").inputValue(), "Keep me", "title kept");
    h.eq(await n.locator("form select[name=shelf]").inputValue(), "reading", "shelf kept");
    h.ok(await n.locator("form input[name=tags][value=science]").isChecked(), "tag kept");
  });
  await h.check("no JS: a good save redirects (303) and the JS tab sees the redraw", async () => {
    await n.goto(h.base + "/");
    const before = await count(a.url().includes("/books/") ? b : a);
    const f = n.locator("form[data-tc-form]");
    await fill(n, f, { title: "Plain", pages: "5", shelf: "done" });
    await f.locator("button[type=submit]").click();
    await n.waitForLoadState();
    h.ok((await n.locator(".books").textContent()).includes("Plain"), "listed after redirect");
    await h.until(async () => (await count(b)) !== before, "JS tab redraw");
  });
  await h.check("no JS: resubmitting the same page saves once", async () => {
    await n.goto(h.base + "/");
    const f = n.locator("form[data-tc-form]");
    await fill(n, f, { title: "Once", pages: "5", shelf: "done" });
    const id = await f.locator("input[name=_tc_submit]").inputValue();
    await f.locator("button[type=submit]").click(); await n.waitForLoadState();
    // the same submission again, as a resubmitted page sends it
    const r = await n.request.post(h.base + "/books", { form: { _tc_submit: id, title: "Once", pages: "5", shelf: "done" }, maxRedirects: 0 });
    h.eq(r.status(), 303, "repeat answers the first redirect");
    await n.goto(h.base + "/");
    h.eq(await n.locator(".books li", { hasText: "Once" }).count(), 1, "saved once");
  });
};
