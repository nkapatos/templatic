module.exports = async (h) => {
  const a = await h.page(); const b = await h.page({ tag: "js2" });
  await h.open(a, "/"); await h.open(b, "/");
  await h.check("JS: Add updates the shared count on both and the tally only on the caller", async () => {
    const tallyB = await b.locator("main").innerHTML();
    const f = a.locator("form[data-tc-form]");
    const before = await b.locator("main").textContent();
    await f.locator("button[type=submit]").click();
    await h.until(async () => (await b.locator("main").textContent()) !== before, "b changes");
  });
};
