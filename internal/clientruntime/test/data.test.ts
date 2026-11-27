import { beforeEach, describe, expect, test, vi } from "vitest";

import { configScript } from "./support/config";
import { stubFetch } from "./support/fetch";
import { frame } from "./support/frame";

let data: typeof import("../ts/data");
let props: typeof import("../ts/props");
let sub: typeof import("../ts/subscribe");
let tc: typeof import("../ts/tc");

beforeEach(async () => {
	vi.useFakeTimers();
	vi.spyOn(console, "error").mockImplementation(() => {});
	vi.resetModules();
	await import("../ts/live");
	data = await import("../ts/data");
	props = await import("../ts/props");
	sub = await import("../ts/subscribe");
	tc = await import("../ts/tc");
	tc.registerBinds({ binds: { "x-log": [{ topic: "log", field: "rows" }] } });
});

type Row = { id: number; text: string };
const row = (id: number): Row => ({ id, text: `r${id}` });
const snap = (ver: number, ids: number[], extra: Record<string, unknown> = {}) => ({
	tc: "snapshot",
	ver,
	...extra,
	items: ids.map((id) => ({ k: `r${id}`, v: row(id) })),
});
const up = (id: number, text = `r${id}`) => ({ op: "u" as const, k: `r${id}`, v: { id, text } });

// host renders a bound island the way the server does: its props, the snapshot in the bound field, in a script.
function host(p: unknown): Element {
	document.body.insertAdjacentHTML(
		"beforeend",
		`<x-log data-tc-island="x-log"><script type="application/tc-props">${JSON.stringify(p)}</script></x-log>`,
	);
	return document.body.lastElementChild!;
}

// bound records what the runtime pushes into the island's bound fields.
function bound(el: Element): Array<{ field: string; v: unknown }> {
	const pushes: Array<{ field: string; v: unknown }> = [];
	(el as unknown as { tcBind: (field: string, v: unknown) => void }).tcBind = (field, v) => pushes.push({ field, v });
	return pushes;
}

const ids = (items: readonly unknown[]) => (items as Row[]).map((r) => r.id);

describe("a bound snapshot", () => {
	test("reaches the island as its items, and other fields pass through", () => {
		const el = host({ title: "Log", rows: snap(3, [3, 2, 1]) });
		const p = props.readProps<{ title: string; rows: Row[] }>(el)!;
		expect(p.title).toBe("Log");
		expect(ids(p.rows)).toEqual([3, 2, 1]);
		expect(data.collection<Row>("log").get("r2")).toEqual(row(2));
	});

	test("a field not bound, or a bound field without the marker, is left alone", () => {
		const el = host({ rows: { ver: 1, items: [] }, other: snap(1, [1]) });
		const p = props.readProps<{ rows: unknown; other: unknown }>(el)!;
		expect(p.rows).toEqual({ ver: 1, items: [] });
		expect(p.other).toEqual(snap(1, [1]));
	});

	test("an older snapshot does not roll back what the page already holds", async () => {
		props.readProps(host({ rows: snap(1, [1]) }));
		await frame({ kind: "ops", topic: "log", ver: 2, ops: [up(2)] });
		const p = props.readProps<{ rows: Row[] }>(host({ rows: snap(1, [1]) }))!;
		expect(ids(p.rows)).toEqual([2, 1]);
	});

	test("the page reports it at the snapshot's version when its stream connects", () => {
		props.readProps(host({ rows: snap(7, [1]) }));
		expect(sub.heldParam()).toBe("log:7");
	});

	test("seeded after the stream connected but before ready, it is subscribed once ready", async () => {
		document.body.innerHTML = configScript();
		const calls = stubFetch({ status: 204 });
		expect(sub.heldParam()).toBe("");
		props.readProps(host({ rows: snap(4, [1]) }));
		tc.markReady();
		await vi.runAllTimersAsync();
		expect(calls.map((c) => c.json())).toEqual([{ sid: "test-sid", subscribe: { log: 4 } }]);
	});
});

describe("ops", () => {
	test("insert at the head, replace in place, remove, and trim the oldest", async () => {
		props.readProps(host({ rows: snap(2, [2, 1], { limit: 3 }) }));
		await frame({ kind: "ops", topic: "log", ver: 6, ops: [up(3), up(1, "edited"), { op: "d", k: "r2" }, up(4)] });
		const c = data.collection<Row>("log");
		expect(ids(c.items())).toEqual([4, 3, 1]);
		expect(c.get("r1")!.text).toBe("edited");
		await frame({ kind: "ops", topic: "log", ver: 7, ops: [up(5)] });
		expect(ids(c.items())).toEqual([5, 4, 3]);
	});

	test("a tail collection appends and trims from the top", async () => {
		props.readProps(host({ rows: snap(2, [1, 2], { tail: true, limit: 2 }) }));
		await frame({ kind: "ops", topic: "log", ver: 3, ops: [up(3)] });
		expect(ids(data.collection("log").items())).toEqual([2, 3]);
	});

	test("ops the page holds are skipped; a gap catches up from the version it holds", async () => {
		document.body.innerHTML = configScript();
		tc.markReady();
		const calls = stubFetch({ status: 204 });
		props.readProps(host({ rows: snap(3, [3, 2, 1]) }));
		await frame({ kind: "ops", topic: "log", ver: 4, ops: [up(3, "old"), up(4)] });
		expect(data.collection<Row>("log").get("r3")!.text).toBe("r3");
		await frame({ kind: "ops", topic: "log", ver: 9, ops: [up(9)] });
		await vi.runAllTimersAsync();
		expect(ids(data.collection("log").items())).toEqual([4, 3, 2, 1]);
		expect(calls.map((c) => c.json()).at(-1)).toEqual({ sid: "test-sid", subscribe: { log: 4 } });
	});

	test("a reset replaces the whole set", async () => {
		props.readProps(host({ rows: snap(3, [3, 2, 1]) }));
		await frame({ kind: "reset", topic: "log", ver: 20, items: snap(20, [20]).items });
		expect(ids(data.collection("log").items())).toEqual([20]);
	});
});

describe("delivery", () => {
	test("bound fields get one new array per frame, however many batches arrived in it", async () => {
		const el = host({ rows: snap(1, [1]) });
		const first = props.readProps<{ rows: Row[] }>(el)!.rows;
		const pushes = bound(el);
		await frame({ kind: "ops", topic: "log", ver: 2, ops: [up(2)] }, { kind: "ops", topic: "log", ver: 3, ops: [up(3)] });
		expect(pushes).toHaveLength(1);
		expect(pushes[0]!.field).toBe("rows");
		expect(ids(pushes[0]!.v as Row[])).toEqual([3, 2, 1]);
		expect(pushes[0]!.v).not.toBe(first);
		expect(ids(first)).toEqual([1]);
	});

	test("subscribers get the items, then each frame's ops, and null ops after a reset", async () => {
		props.readProps(host({ rows: snap(1, [1]) }));
		const seen: Array<{ ids: number[]; ops: unknown }> = [];
		const stop = data.collection<Row>("log").subscribe((items, ops) => seen.push({ ids: ids(items), ops }));
		await frame({ kind: "ops", topic: "log", ver: 3, ops: [up(2), { op: "d", k: "r1" }] });
		await frame({ kind: "reset", topic: "log", ver: 9, items: snap(9, [9]).items });
		stop();
		await frame({ kind: "ops", topic: "log", ver: 10, ops: [up(10)] });
		expect(seen).toEqual([
			{ ids: [1], ops: null },
			{ ids: [2], ops: [up(2), { op: "d", k: "r1" }] },
			{ ids: [9], ops: null },
		]);
	});

	test("a subscriber that comes before the island waits for it", () => {
		const seen: number[][] = [];
		data.collection<Row>("log").subscribe((items) => seen.push(ids(items)));
		expect(data.collection("log").items()).toEqual([]);
		props.readProps(host({ rows: snap(1, [1]) }));
		expect(seen).toEqual([[1]]);
	});

	test("ops for a topic the page holds no store for are left to rendered containers", async () => {
		await frame({ kind: "ops", topic: "log", ver: 1, ops: [up(1)] });
		expect(data.collection("log").items()).toEqual([]);
	});
});
