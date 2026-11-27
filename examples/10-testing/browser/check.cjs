// The browser check: builds and starts the app, opens it in Chromium with
// JavaScript on (two pages, one behind a relay) and off, and fails on a
// wrong page, a wrong window.tcState() or any console error.
// Run after `go tool templatic generate`: `npm run check-browser`.
const { spawn, execFileSync } = require("node:child_process");
const fs = require("node:fs");
const net = require("node:net");
const os = require("node:os");
const path = require("node:path");
const { chromium } = require("playwright");
const { relay } = require("./relay.cjs");

const appDir = path.join(__dirname, "..");
const timeout = 10_000;

function check(ok, what) {
	if (!ok) throw new Error(`check failed: ${what}`);
	console.log(`ok   ${what}`);
}

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

// startApp builds the app into dir and runs it on addr until stopped.
async function startApp(dir, addr) {
	const bin = path.join(dir, "app");
	execFileSync("go", ["build", "-o", bin, "."], { cwd: appDir, stdio: "inherit" });
	const proc = spawn(bin, [], { cwd: appDir, env: { ...process.env, ADDR: addr }, stdio: ["ignore", "inherit", "inherit"] });
	const deadline = Date.now() + timeout;
	for (;;) {
		try {
			if ((await fetch(`http://${addr}/`)).ok) return proc;
		} catch {}
		if (Date.now() > deadline) throw new Error(`the app did not answer on ${addr}`);
		await new Promise((r) => setTimeout(r, 100));
	}
}

// watch records the page's console errors and uncaught exceptions in
// errors. The browser logs a failed load as a console error too; one whose
// URL contains a string in expected.get(page) is a failure the check caused
// on purpose (a cut stream, a refused post) and is not recorded.
function watch(page, name, errors, expected) {
	expected.set(page, []);
	page.on("console", (m) => {
		if (m.type() !== "error") return;
		const url = m.location().url ?? "";
		if (m.text().startsWith("Failed to load resource") && expected.get(page).some((part) => url.includes(part))) return;
		errors.push(`${name}: console: ${m.text()} (${url})`);
	});
	page.on("pageerror", (e) => errors.push(`${name}: ${e.message}`));
	return page;
}

const live = (page) => page.evaluate(() => window.tcState().live);
const text = (page, sel) => page.locator(sel).innerText();

// waitLive waits until the page's live record satisfies pred.
function waitLive(page, pred, arg) {
	return page.waitForFunction(pred, arg, { timeout });
}

// waitText waits until sel's text contains want.
function waitText(page, sel, want) {
	return page.waitForFunction(([s, w]) => document.querySelector(s)?.textContent.includes(w), [sel, want], { timeout });
}

async function add(page, n) {
	await page.fill('input[name="n"]', String(n));
	await page.click('form.add button[type="submit"]');
}

async function main() {
	const dir = fs.mkdtempSync(path.join(os.tmpdir(), "templatic-check-"));
	const addr = `127.0.0.1:${process.env.PORT || (await freePort())}`;
	const errors = [];
	const expected = new Map();
	let app, r, browser;
	try {
		app = await startApp(dir, addr);
		r = await relay(addr);
		browser = await chromium.launch();

		// Two pages with JavaScript: a behind the relay, b direct.
		const js = await browser.newContext();
		const a = watch(await js.newPage(), "a", errors, expected);
		const b = watch(await js.newPage(), "b", errors, expected);
		await a.goto(`http://${r.addr}/`);
		await b.goto(`http://${addr}/`);
		// Act only once both pages are connected: an update sent before a
		// page connects does not reach it (#39).
		for (const p of [a, b]) await waitLive(p, () => window.tcState().live.connection === "open");
		check(true, "both pages connected");

		await add(a, 2);
		for (const p of [a, b]) await waitText(p, "#count", "2");
		check(true, "a's add shows on both pages");
		await waitText(a, "#tally", "added 1 times, last 2");
		check((await text(b, "#tally")).includes("not added yet"), "only a's tally changed");
		await waitText(b, "[data-tc-collection]", "+2 → 2");
		check(true, "the add is logged on b");

		// Cut a's connection, add on b while a is away, then restore it.
		expected.get(a).push("/live/stream");
		r.cut();
		await waitLive(a, () => window.tcState().live.connection === "torn");
		check((await live(a)).disconnects === 1, "a recorded one disconnect");
		await add(b, 3);
		await waitText(b, "#count", "5");
		check((await text(a, "#count")) === "2", "a missed the add while cut off");
		r.restore();
		await waitLive(a, () => {
			const l = window.tcState().live;
			return l.connection === "open" && l.resumes === 1;
		});
		check(true, "a reconnected and resumed");
		await waitText(a, "#count", "5");
		await waitText(a, "[data-tc-collection]", "+3 → 5");
		check(true, "a caught up: the count and the log caught up");

		// Without JavaScript: the form posts, the server redirects, and the
		// live pages follow.
		const nojs = await browser.newContext({ javaScriptEnabled: false });
		const c = watch(await nojs.newPage(), "c", errors, expected);
		await c.goto(`http://${addr}/`);
		check((await text(c, "#count")) === "5", "the page without JS renders the count");
		await add(c, 1);
		await c.waitForURL(`http://${addr}/`);
		check((await text(c, "#count")) === "6", "the no-JS add redirected to the new count");
		for (const p of [a, b]) await waitText(p, "#count", "6");
		check(true, "the no-JS add reached the live pages");
		expected.get(c).push("/add");
		const [res] = await Promise.all([c.waitForResponse((res) => res.url().endsWith("/add")), add(c, 500)]);
		check(res.status() === 422, "a refused no-JS add answers 422");
		check((await text(c, "[data-tc-field-error]")).includes("between -100 and 100"), "and shows the field error");

		for (const [name, p] of [["a", a], ["b", b]]) {
			const s = await p.evaluate(() => window.tcState());
			check(Object.keys(s.lastErrors).length === 0, `${name}: tcState has no errors`);
		}
		check(errors.length === 0, `no console errors${errors.length ? ":\n  " + errors.join("\n  ") : ""}`);
	} finally {
		await browser?.close();
		await r?.close();
		app?.kill();
		fs.rmSync(dir, { recursive: true, force: true });
	}
}

main().catch((err) => {
	console.error(err.message);
	process.exitCode = 1;
});
