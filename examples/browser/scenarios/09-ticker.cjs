module.exports = async (h) => {
  const a = await h.page(); await h.open(a, "/");
  await h.check("JS: prices and as-of update; rows carry data-dir", async () => {
    const asof = await a.locator(".asof").textContent();
    await h.until(async () => (await a.locator(".asof").textContent()) !== asof, "asof ticks", 2000);
    await h.until(async () => (await a.locator("ol.board li[data-dir]").count()) > 0, "data-dir set");
    const px = await a.locator("ol.board .px").allTextContents();
    await h.until(async () => JSON.stringify(await a.locator("ol.board .px").allTextContents()) !== JSON.stringify(px), "prices move", 2000);
  });
};
