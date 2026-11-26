import { beforeEach, expect, test, vi } from "vitest";

import { frame } from "./support/frame";

let tc: typeof import("../ts/tc");
let context: typeof import("../ts/context");

beforeEach(async () => {
	vi.useFakeTimers();
	vi.spyOn(console, "error").mockImplementation(() => {});
	vi.resetModules();
	tc = await import("../ts/tc");
	context = await import("../ts/context");
	document.body.innerHTML = `
<div data-tc-region="panel"><p>old</p></div>
<div data-tc-stream="feed"></div>
<span data-tc-cell="n">0</span>
`;
});

test("tcState records boot phases and their failures visibly", () => {
	tc.tcPhase("live");
	tc.tcPhase("forms", new Error("boom"));
	const s = tc.tcState();
	expect(s.phases["live"]).toBe("ok");
	expect(s.phases["forms"]).toBe("failed");
	expect(s.lastErrors["forms"]).toBe("boom");
	expect(s.sid).toBe(tc.sessionID());
	expect(s.ready).toBe(false);
});

test("bus: cell fires after the holder is written", async () => {
	const seen: string[] = [];
	tc.on("cell", (d) => seen.push(`${d.topic}=${d.value}|${document.querySelector('[data-tc-cell="n"]')?.textContent}`));
	await frame({ kind: "cell", topic: "n", v: 7 });
	expect(seen).toEqual(["n=7|7"]);
});

test("bus: swap:before precedes the morph, swap:after follows it", async () => {
	const seen: string[] = [];
	const text = () => document.querySelector('[data-tc-region="panel"]')?.textContent ?? "";
	tc.on("swap:before", (d) => seen.push(`before:${d.topic}:${text()}`));
	tc.on("swap:after", (d) => seen.push(`after:${d.topic}:${text()}`));
	await frame({ kind: "swap", topic: "panel", html: "<p>new</p>" });
	expect(seen).toEqual(["before:panel:old", "after:panel:new"]);
});

test("bus: append fires per inserted item, after insertion", async () => {
	const seen: string[] = [];
	tc.on("append", (d) => seen.push(`${d.key}:${document.querySelector(`[data-tc-item="${d.key}"]`) ? "in" : "missing"}`));
	await frame({ kind: "append", topic: "feed", items: [{ k: "a", html: '<p data-tc-item="a">A</p>' }, { k: "b", html: '<p data-tc-item="b">B</p>' }] });
	expect(seen).toEqual(["a:in", "b:in"]);
});

test("bus: a throwing listener is contained and recorded", async () => {
	const seen: unknown[] = [];
	tc.on("cell", () => {
		throw new Error("boom");
	});
	tc.on("cell", (d) => seen.push(d.value));
	await frame({ kind: "cell", topic: "n", v: 1 });
	expect(seen).toEqual([1]);
	expect(tc.tcState().lastErrors.listener ?? "").toContain("boom");
});

test("bus: unsubscribe stops delivery", async () => {
	const seen: unknown[] = [];
	const off = tc.on("cell", (d) => seen.push(d.value));
	await frame({ kind: "cell", topic: "n", v: 1 });
	off();
	await frame({ kind: "cell", topic: "n", v: 2 });
	expect(seen).toEqual([1]);
});

test("cell(): subscribe after a delivery gets the latest, then updates", async () => {
	await frame({ kind: "cell", topic: "late", v: "x" });
	const c = tc.cell<string>("late");
	expect(c.get()).toBe("x");
	const seen: string[] = [];
	c.subscribe((v) => seen.push(v));
	await frame({ kind: "cell", topic: "late", v: "y" }, { kind: "cell", topic: "other", v: "z" });
	expect(seen).toEqual(["x", "y"]);
});

test("island ctx: dispose ends subscriptions, aborts signal, emits unmount", async () => {
	const host = document.querySelector('[data-tc-region="panel"]')!;
	const events: string[] = [];
	tc.on("island:unmount", (d) => events.push(`unmount:${d.tag}`));
	const m = context.mountIsland("x-test", host);
	const seen: unknown[] = [];
	m.ctx.cell("ctx.n").subscribe((v) => seen.push(v));
	m.ctx.on("cell", () => seen.push("bus"));
	await frame({ kind: "cell", topic: "ctx.n", v: 1 });
	m.dispose();
	m.dispose();
	await frame({ kind: "cell", topic: "ctx.n", v: 2 });
	expect(m.ctx.signal.aborted).toBe(true);
	expect(seen).toEqual([1, "bus"]);
	expect(events).toEqual(["unmount:x-test"]);
});

test("a throwing ready listener does not stop the others or the ready event", () => {
	const log: string[] = [];
	tc.onReady(() => log.push("a"));
	tc.onReady(() => {
		throw new Error("boom");
	});
	tc.onReady(() => log.push("c"));
	const stop = tc.on("ready", () => log.push("event"));
	tc.markReady();
	stop();
	expect(log).toEqual(["a", "c", "event"]);
	expect(tc.tcState().lastErrors.listener).toContain("boom");
});
