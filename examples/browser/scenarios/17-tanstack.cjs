module.exports = async (h) => {
  const a = await h.page(); const urls = [];
  h.allow(a, (r) => r.request().method() === "POST" && r.status() === 400); a.on("request", (r) => urls.push(r.url()));
  await h.open(a, "/");
  await h.check("JS: the log island virtualizes and grows with ops; / loads no Table/Form code", async () => {
    await h.until(async () => (await a.locator("log-list li.row").count()) > 0, "rows rendered");
    const n = await a.locator("log-list li.row").count(); h.ok(n < 60, "virtualized: " + n);
    h.ok(!urls.some((u) => /order-table|signup-form/.test(u)), "no other island code");
    const first = await a.locator("log-list").textContent();
    await h.until(async () => (await a.locator("log-list").textContent()) !== first, "new rows arrive", 5000);
  });
  await h.check("JS: the order table sorts in the browser and keeps its sort across pushes", async () => {
    await a.getByRole("link", { name: "Table" }).click(); await a.waitForURL(/table/); await h.ready(a);
    const t = a.locator("order-table"); await h.until(async () => (await t.locator("tbody tr").count()) > 0, "rows");
    const th = t.locator("thead th").nth(3);
    await th.locator("button").click();
    await h.until(async () => (await th.getAttribute("aria-sort")) !== "none", "sorted");
    const sort1 = await th.getAttribute("aria-sort");
    await t.locator("input").first().fill("a").catch(() => {});
    await a.waitForTimeout(3500); // the server pushes status changes every 3s
    h.eq(await th.getAttribute("aria-sort"), sort1, "sort kept across a push");
  });
  await h.check("JS: the signup island shows client errors, then the server's 'taken' on username", async () => {
    await a.getByRole("link", { name: "Sign up" }).click(); await a.waitForURL(/signup/); await h.ready(a);
    const f = a.locator("signup-form");
    await h.until(async () => (await f.locator("input[name=username]").count()) === 1, "island mounted");
    await f.locator("input[name=email]").fill("x"); await f.locator("input[name=email]").blur();
    await h.until(async () => /email/i.test(await f.textContent()), "client email error");
    await f.locator("input[name=username]").fill("ada"); await f.locator("input[name=email]").fill("ada@example.com");
    const plan = f.locator("select[name=plan]"); if (await plan.count()) await plan.selectOption({ index: 1 });
    await f.locator("button[type=submit]").click();
    await h.until(async () => /taken/i.test(await f.textContent()), "server error on username");
    await f.locator("input[name=username]").fill("newbie" + Date.now() % 1000);
    await f.locator("button[type=submit]").click();
    await h.until(async () => !/taken/i.test(await f.textContent()), "accepted");
  });
  h.allow(a, (r) => r.request().method() === "POST" && r.status() === 400);
  const n = await h.page({ js: false });
  h.allow(n, (r) => r.status() === 422);
  await h.check("no JS: the signup fallback form posts; taken re-renders with the error", async () => {
    await n.goto(h.base + "/signup");
    await n.locator("input[name=username]").fill("grace"); await n.locator("input[name=email]").fill("g@example.com");
    const plan = n.locator("select[name=plan]"); if (await plan.count()) await plan.selectOption({ index: 1 });
    await n.locator("button[type=submit]").click(); await n.waitForLoadState();
    h.ok(/taken/i.test(await n.locator("main").textContent()), "taken shown");
  });
};
