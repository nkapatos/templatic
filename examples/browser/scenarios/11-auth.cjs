module.exports = async (h) => {
  const login = async (p, name) => {
    await p.goto(h.base + "/login");
    await p.locator("input[name=name]").fill(name); await p.locator("input[name=password]").fill(name);
    await p.locator("button[type=submit]").click(); await p.waitForURL(h.base + "/"); if (p.tag !== "nojs") await h.ready(p);
  };
  const alice = await h.page({ tag: "alice" }); const bob = await h.page({ tag: "bob" });
  await h.check("the login page loads without errors", async () => { await alice.goto(h.base + "/login"); await alice.waitForTimeout(1500); });
  await login(alice, "alice"); await login(bob, "bob");
  h.allow(bob, (r) => r.status() === 403 && r.url().endsWith("/notice"));
  await h.check("JS: admin posts a notice; the viewer sees it live", async () => {
    const f = alice.locator("form[data-tc-form]").filter({ has: alice.locator("input[name=text]") });
    await f.locator("input[name=text]").fill("hello team"); await f.locator("button").click();
    await h.until(async () => (await bob.locator(".notice").textContent()).includes("hello team"), "bob sees notice");
  });
  await h.check("JS: the viewer has no notice form, and posting by hand is 403", async () => {
    h.eq(await bob.locator("input[name=text]").count(), 0, "no form");
    const st = await bob.evaluate(async () => (await fetch("/notice", { method: "POST", body: new URLSearchParams({ text: "x" }) })).status);
    h.eq(st, 403, "status");
  });
  await h.check("JS: the viewer waves; the admin's activity updates", async () => {
    const before = await alice.locator("main").textContent();
    await bob.getByRole("button", { name: "Wave" }).click();
    await h.until(async () => (await alice.locator("main").textContent()) !== before, "alice sees wave");
  });
  await h.check("JS: logout ends that browser's other tabs, not the other user's", async () => {
    const bob2 = await bob.context().newPage(); bob2.tag = "bob2"; await bob2.goto(h.base + "/"); await h.ready(bob2);
    await bob.getByRole("button", { name: "Log out" }).click(); await bob.waitForURL(/\/login/);
    await h.until(async () => /\/login/.test(bob2.url()), "bob's other tab goes to login", 6000);
    h.ok(alice.url() === h.base + "/", "alice stays");
    await alice.getByRole("button", { name: "Wave" }).click();
  });
  const n = await h.page({ js: false });
  h.allow(n, (r) => [403, 422].includes(r.status()));
  await h.check("no JS: login, wave and notice post work and redirect back", async () => {
    await login(n, "alice");
    const f = n.locator("form[data-tc-form]").filter({ has: n.locator("input[name=text]") });
    await f.locator("input[name=text]").fill("plain notice"); await f.locator("button").click(); await n.waitForLoadState();
    h.ok((await n.locator(".notice").textContent()).includes("plain notice"), "rendered");
    await h.until(async () => (await alice.locator(".notice").textContent()).includes("plain notice"), "JS tab live");
  });
};
