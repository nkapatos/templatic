// The examples' browser check: builds and starts each example, then runs
// scenarios/<example>.cjs against it in Chromium, with and without
// JavaScript. A scenario fails on a broken behavior, a console error, an
// uncaught exception or an unexpected 4xx/5xx. Run after each example's
// generate step: `node run.cjs [example...]` (all by default).
const { spawn, execFileSync } = require("node:child_process");
const fs = require("node:fs");
const net = require("node:net");
const os = require("node:os");
const path = require("node:path");
const { chromium } = require("playwright");

const root = path.join(__dirname, "..");
const bins = fs.mkdtempSync(path.join(os.tmpdir(), "tc-examples-"));

// freePort asks the OS for a port that is free now.
function freePort() {
  return new Promise((ok, fail) => {
    const s = net.createServer().on("error", fail);
    s.listen(0, "127.0.0.1", () => {
      const { port } = s.address();
      s.close(() => ok(port));
    });
  });
}

async function start(d) {
  const dir = path.join(root, d), bin = path.join(bins, d), addr = `127.0.0.1:${await freePort()}`;
  execFileSync("go", ["build", "-o", bin, "."], { cwd: dir, stdio: "pipe" });
  const proc = spawn(bin, [], { cwd: dir, env: { ...process.env, ADDR: addr }, stdio: ["ignore", "pipe", "pipe"] });
  proc.log = ""; proc.stdout.on("data", b => proc.log += b); proc.stderr.on("data", b => proc.log += b);
  for (let i = 0; i < 100; i++) { try { await fetch(`http://${addr}/`, { redirect: "manual" }); break; } catch { await new Promise(r => setTimeout(r, 100)); } }
  return { proc, base: `http://${addr}` };
}

(async () => {
  const browser = await chromium.launch();
  let failed = 0;
  const all = fs.readdirSync(path.join(__dirname, "scenarios")).map((f) => f.replace(/\.cjs$/, "")).sort();
  for (const d of process.argv.length > 2 ? process.argv.slice(2) : all) {
    const { proc, base } = await start(d);
    const problems = [], results = [];
    const watch = (page, tag) => {
      page.on("console", m => { if (m.text().startsWith("Failed to load resource")) return; if (m.type() === "error" || m.type() === "warning") problems.push(`${tag} console.${m.type()}: ${m.text()}`); });
      page.on("pageerror", e => problems.push(`${tag} pageerror: ${e.message}`));
      page.on("response", r => { if (r.status() >= 400 && !page.allow?.(r)) problems.push(`${tag} HTTP ${r.status()} ${r.request().method()} ${r.url().replace(base, "")}`); });
    };
    const ctxs = [];
    const h = {
      base,
      async page({ js = true, tag = js ? "js" : "nojs" } = {}) {
        const ctx = await browser.newContext({ javaScriptEnabled: js }); ctxs.push(ctx);
        const p = await ctx.newPage(); watch(p, tag); p.tag = tag; return p;
      },
      async open(p, url) { await p.goto(base + url); if (p.tag !== "nojs") await h.ready(p); },
      async ready(p) { await p.waitForFunction(() => typeof tcState === "function" && tcState().ready, null, { timeout: 5000 }); },
      async check(name, fn) {
        try { await fn(); results.push(`ok   ${name}`); }
        catch (e) { failed++; results.push(`FAIL ${name}: ${String(e.message).split("\n")[0]}`); }
      },
      eq(got, want, what) { if (JSON.stringify(got) !== JSON.stringify(want)) throw new Error(`${what}: got ${JSON.stringify(got)}, want ${JSON.stringify(want)}`); },
      ok(c, what) { if (!c) throw new Error(what); },
      async until(fn, what, ms = 4000) { const t = Date.now() + ms; for (;;) { try { if (await fn()) return; } catch {} if (Date.now() > t) throw new Error("timed out: " + what); await new Promise(r => setTimeout(r, 50)); } },
      allow(p, pred) { p.allow = pred; },
    };
    try { await require(path.join(__dirname, "scenarios", d + ".cjs"))(h); }
    catch (e) { failed++; results.push(`FAIL (scenario crashed): ${e.message.split("\n")[0]}`); }
    for (const c of ctxs) await c.close();
    proc.kill();
    console.log(`## ${d}`); for (const r of results) console.log("  " + r);
    if (problems.length) { failed++; console.log("  problems:"); for (const p of [...new Set(problems)]) console.log("    " + p.slice(0, 220)); }
    if (/panic|fatal/i.test(proc.log)) { failed++; console.log("  server log:\n" + proc.log.slice(-1500)); }
  }
  await browser.close();
  fs.rmSync(bins, { recursive: true, force: true });
  process.exitCode = failed ? 1 : 0;
})();
