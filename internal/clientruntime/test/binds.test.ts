import { beforeEach, expect, test, vi } from "vitest";

import { frame, settle } from "./support/frame";

let live: typeof import("../ts/live");
let tc: typeof import("../ts/tc");

beforeEach(async () => {
	vi.useFakeTimers();
	vi.resetModules();
	live = await import("../ts/live");
	tc = await import("../ts/tc");
});

type Push = { f: string; v: unknown };

function recordBinds(host: Element, into: Push[]): void {
	(host as unknown as { tcBind: (f: string, v: unknown) => void }).tcBind = (f, v) => {
		into.push({ f, v });
	};
}

function register(tag: string, entries: Array<{ topic: string; field: string }>): void {
	tc.registerBinds({ binds: { [tag]: entries } });
}

function host(tag: string): Element {
	const el = document.querySelector(`[data-tc-island="${tag}"]`);
	if (!el) throw new Error(`fixture missing host ${tag}`);
	return el;
}

test("a bind delivers the cell value into the bound host, holders still written", async () => {
	register("x-graph-a", [{ topic: "g.focus-a", field: "focus" }]);
	document.body.innerHTML = `
		<div data-tc-region="ga">
			<span data-tc-cell="g.focus-a">seed</span>
			<x-graph-a data-tc-island="x-graph-a"></x-graph-a>
		</div>`;
	const recs: Push[] = [];
	recordBinds(host("x-graph-a"), recs);

	await frame({ kind: "cell", topic: "g.focus-a", v: { id: "n1" } });
	expect(recs).toEqual([{ f: "focus", v: { id: "n1" } }]);
	expect(document.querySelector('[data-tc-cell="g.focus-a"]')?.textContent).toBe('{"id":"n1"}');

	await frame({ kind: "cell", topic: "g.unbound-a", v: 7 });
	expect(recs).toHaveLength(1);
});

test("coalesced cell writes deliver one push with the latest value", async () => {
	register("x-graph-b", [{ topic: "g.focus-b", field: "focus" }]);
	document.body.innerHTML = `
		<div>
			<span data-tc-cell="g.focus-b"></span>
			<x-graph-b data-tc-island="x-graph-b"></x-graph-b>
		</div>`;
	const recs: Push[] = [];
	recordBinds(host("x-graph-b"), recs);

	live.apply({ kind: "cell", topic: "g.focus-b", v: "first" });
	live.apply({ kind: "cell", topic: "g.focus-b", v: "second" });
	await settle();
	expect(recs).toEqual([{ f: "focus", v: "second" }]);
});

test("hosts without tcBind are skipped silently (un-upgraded islands)", async () => {
	register("x-graph-c", [{ topic: "g.focus-c", field: "focus" }]);
	document.body.innerHTML = `
		<div>
			<span data-tc-cell="g.focus-c"></span>
			<x-graph-c data-tc-island="x-graph-c"></x-graph-c>
		</div>`;
	await frame({ kind: "cell", topic: "g.focus-c", v: 1 });
	expect(document.querySelector('[data-tc-cell="g.focus-c"]')?.textContent).toBe("1");
});

test("after a remounting swap, the next publish reaches the fresh host", async () => {
	register("x-graph-d", [{ topic: "g.focus-d", field: "focus" }]);
	document.body.innerHTML = `
		<div data-tc-region="gd">
			<x-graph-d data-tc-island="x-graph-d"></x-graph-d>
		</div>`;
	const oldHost = host("x-graph-d");
	const oldRecs: Push[] = [];
	recordBinds(oldHost, oldRecs);
	await frame({ kind: "cell", topic: "g.focus-d", v: "pre" });
	expect(oldRecs).toEqual([{ f: "focus", v: "pre" }]);

	await frame({ kind: "swap", topic: "gd", html: `<x-graph-d data-tc-island="x-graph-d"></x-graph-d>` });
	const freshHost = host("x-graph-d");
	expect(freshHost).not.toBe(oldHost);
	const freshRecs: Push[] = [];
	recordBinds(freshHost, freshRecs);
	await frame({ kind: "cell", topic: "g.focus-d", v: "post" });
	expect(freshRecs).toEqual([{ f: "focus", v: "post" }]);
	expect(oldRecs).toHaveLength(1);
});

test("applied cell values latch for late island subscribers", async () => {
	await frame({ kind: "cell", topic: "g.seed-e", v: { id: "fresh" } });
	const last = tc.latestCell("g.seed-e");
	expect(last.known).toBe(true);
	expect(last.value).toEqual({ id: "fresh" });
	expect(tc.latestCell("g.never-e").known).toBe(false);
});
