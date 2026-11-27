import { beforeEach, describe, expect, test, vi } from "vitest";

import { frame, settle } from "./support/frame";
import { recordMorphs } from "./support/morph";

let live: typeof import("../ts/live");
let tc: typeof import("../ts/tc");

beforeEach(async () => {
	vi.useFakeTimers();
	vi.spyOn(console, "error").mockImplementation(() => {});
	vi.resetModules();
	live = await import("../ts/live");
	tc = await import("../ts/tc");
});

const FIXTURE = `
<div data-tc-region="ws">
	<x-editor data-tc-island=""><p class="live">LIVE DOM</p></x-editor>
	<p id="preview">old text</p>
	<span data-tc-cell="ws.welcome">hello</span>
	<div data-tc-region="metrics"><span data-tc-cell="ws.count">0</span></div>
</div>
<div data-tc-region="drops">
	<div data-tc-collection="drops.list" data-tc-ver="1">
		<div data-tc-item="n1"><p>N1</p></div>
	</div>
</div>
<div data-tc-cell="ws.count">mirror</div>
`;

const TAIL_FIXTURE = `
<div data-tc-region="brain">
	<div data-tc-collection="brain.conv" data-tc-ver="1" data-tc-dir="tail">
		<div data-tc-item="m1"><p>M1</p></div>
	</div>
</div>
`;

function mount(html: string): void {
	document.body.innerHTML = html;
}

function regionOf(topic: string): Element {
	const r = document.querySelector(`[data-tc-region="${topic}"]`);
	if (!r) throw new Error(`fixture missing region ${topic}`);
	return r;
}

function containerOf(topic: string): Element {
	const s = document.querySelector(`[data-tc-collection="${topic}"]`);
	if (!s) throw new Error(`fixture missing collection ${topic}`);
	return s;
}

const up = (k: string, body = k.toUpperCase()) => ({ op: "u" as const, k, html: `<div data-tc-item="${k}"><p>${body}</p></div>` });

function itemKeys(container: Element): Array<string | null> {
	return [...container.children].map((c) => c.getAttribute("data-tc-item"));
}

function text(selector: string): string | undefined {
	return document.querySelector(selector)?.textContent ?? undefined;
}

describe("cells", () => {
	test("cells write every holder in one batched frame", async () => {
		mount(FIXTURE);
		live.apply({ kind: "cell", topic: "ws.count", v: 42 });
		live.apply({ kind: "cell", topic: "ws.welcome", v: "hi" });
		expect(vi.getTimerCount()).toBe(1);
		await settle();
		expect([...document.querySelectorAll('[data-tc-cell="ws.count"]')].map((c) => c.textContent)).toEqual(["42", "42"]);
		expect(text('[data-tc-cell="ws.welcome"]')).toBe("hi");
	});

	test("cell writes coalesce to the latest within one frame", async () => {
		mount(FIXTURE);
		await frame({ kind: "cell", topic: "ws.count", v: 1 }, { kind: "cell", topic: "ws.count", v: "two" });
		expect(text('[data-tc-cell="ws.count"]')).toBe("two");
	});

	test("cell values render strings, numbers, objects and null", async () => {
		mount(FIXTURE);
		await frame({ kind: "cell", topic: "ws.count", v: 1.5 }, { kind: "cell", topic: "ws.welcome", v: { a: 1 } });
		expect(text('[data-tc-cell="ws.count"]')).toBe("1.5");
		expect(text('[data-tc-cell="ws.welcome"]')).toBe('{"a":1}');
		await frame({ kind: "cell", topic: "ws.welcome", v: null });
		expect(text('[data-tc-cell="ws.welcome"]')).toBe("");
	});
});

describe("swap", () => {
	test("swap morphs region content and keeps the region marker", async () => {
		const { morphs } = recordMorphs();
		mount(FIXTURE);
		const region = regionOf("ws");
		await frame({ kind: "swap", topic: "ws", html: `<p data-tc-text="preview">new text</p><p>fresh</p>` });
		expect(document.querySelector('[data-tc-region="ws"]')).toBe(region);
		expect(region.querySelector('[data-tc-text="preview"]')?.textContent).toBe("new text");
		expect(morphs).toHaveLength(1);
		expect(morphs[0].style).toBe("innerHTML");
		expect(morphs[0].topic).toBe("ws");
	});

	test("swap pins island hosts first: fresh host lands, old instance detaches", async () => {
		const { morphs } = recordMorphs();
		mount(FIXTURE);
		const region = regionOf("ws");
		const liveHost = region.querySelector("[data-tc-island]")!;
		await frame({ kind: "swap", topic: "ws", html: `<x-editor data-tc-island=""><p class="placeholder">RESEEDED</p></x-editor>` });
		const fresh = region.querySelector("[data-tc-island]");
		expect(fresh).not.toBeNull();
		expect(fresh).not.toBe(liveHost);
		expect(fresh?.textContent).toBe("RESEEDED");
		expect(liveHost.parentNode).toBeNull();
		expect(morphs[0].islandsPresent).toBe(0);
		expect(morphs[0].style).toBe("innerHTML");
	});

	test("morphs serialize across frames in arrival order", async () => {
		const { morphs } = recordMorphs();
		mount(FIXTURE);
		await frame({ kind: "swap", topic: "ws", html: `<span data-tc-cell="ws.count">1</span><div data-tc-region="metrics"><span data-tc-cell="ws.count">m</span></div>` });
		await frame({ kind: "swap", topic: "drops", html: `<div data-tc-collection="drops.list" data-tc-ver="1"><div data-tc-item="x">X</div></div>` });
		await frame(
			{ kind: "swap", topic: "ws", html: `<span data-tc-cell="ws.count">2</span><div data-tc-region="metrics"><span data-tc-cell="ws.count">m</span></div>` },
			{ kind: "swap", topic: "metrics", html: `<span data-tc-cell="ws.count">3</span>` },
		);
		expect(morphs.map((m) => m.topic)).toEqual(["ws", "drops", "ws", "metrics"]);
		expect(text('[data-tc-cell="ws.count"]')).toBe("2");
		expect(text('[data-tc-item="x"]')).toBe("X");
		expect(regionOf("metrics").querySelector('[data-tc-cell="ws.count"]')?.textContent).toBe("3");
	});

	test("swap on a missing region is a no-op", async () => {
		const { morphs } = recordMorphs();
		mount(FIXTURE);
		await frame({ kind: "swap", topic: "nope.gone", html: `<p>anything</p>` });
		expect(morphs).toHaveLength(0);
	});

	test("region-less swaps are dropped, later real swaps still apply", async () => {
		const { morphs } = recordMorphs();
		mount(FIXTURE);
		await frame(
			{ kind: "swap", topic: "no.such.region", html: `<p>dropped</p>` },
			{ kind: "swap", topic: "also.missing", html: `<p>dropped too</p>` },
		);
		expect(morphs).toHaveLength(0);
		expect(text("[data-tc-island]")).toBe("LIVE DOM");
		await frame({ kind: "swap", topic: "metrics", html: `<span data-tc-cell="ws.count">7</span>` });
		expect(regionOf("metrics").textContent).toBe("7");
	});

	test("a failed morph never poisons later swaps (failure containment)", async () => {
		const { failNext } = recordMorphs();
		mount(FIXTURE);
		failNext("injected: the morph runtime fetch failed");
		await frame({ kind: "swap", topic: "metrics", html: `<span data-tc-cell="ws.count">1</span>` });
		expect(regionOf("metrics").textContent).toBe("0");
		await frame({ kind: "swap", topic: "metrics", html: `<span data-tc-cell="ws.count">2</span>` });
		expect(regionOf("metrics").textContent).toBe("2");
		await frame({ kind: "swap", topic: "metrics", html: `<span data-tc-cell="ws.count">3</span>` });
		expect(regionOf("metrics").textContent).toBe("3");
	});

	test("region morphs never sweep hosts outside the swapped region", async () => {
		mount(FIXTURE);
		const host = document.querySelector("[data-tc-island]")!;
		await frame({ kind: "swap", topic: "drops", html: `<div data-tc-collection="drops.list" data-tc-ver="1"><div data-tc-item="z">Z</div></div>` });
		expect(document.querySelector("[data-tc-island]")).toBe(host);
		expect(host.textContent).toBe("LIVE DOM");
	});
});

describe("collections", () => {
	test("ops insert at the head and update in place", async () => {
		mount(FIXTURE);
		await frame({ kind: "ops", topic: "drops.list", ver: 4, ops: [up("n3"), up("n4"), up("n1", "N1b")] });
		expect(itemKeys(containerOf("drops.list"))).toEqual(["n4", "n3", "n1"]);
		expect(text('[data-tc-item="n1"]')).toBe("N1b");
		expect(containerOf("drops.list").getAttribute("data-tc-ver")).toBe("4");
	});

	test("a batch delivered twice applies once", async () => {
		mount(FIXTURE);
		await frame({ kind: "ops", topic: "drops.list", ver: 2, ops: [up("n2", "first")] });
		await frame({ kind: "ops", topic: "drops.list", ver: 2, ops: [up("n2", "again")] });
		expect(containerOf("drops.list").querySelectorAll('[data-tc-item="n2"]')).toHaveLength(1);
		expect(text('[data-tc-item="n2"]')).toBe("first");
	});

	test("batches in one frame apply in arrival order, onSwapped once per batch", async () => {
		mount(FIXTURE);
		const swapped = vi.fn();
		live.onSwapped(swapped);
		await frame(
			{ kind: "ops", topic: "drops.list", ver: 2, ops: [up("n5")] },
			{ kind: "ops", topic: "drops.list", ver: 4, ops: [up("n6"), { op: "d", k: "n1" }] },
		);
		expect(itemKeys(containerOf("drops.list"))).toEqual(["n6", "n5"]);
		expect(swapped).toHaveBeenCalledTimes(2);
	});

	test("ops on a missing container are a no-op", async () => {
		mount(FIXTURE);
		const swapped = vi.fn();
		live.onSwapped(swapped);
		await frame({ kind: "ops", topic: "nope.list", ver: 1, ops: [up("x")] });
		expect(swapped).not.toHaveBeenCalled();
	});

	test("a swap then an op on a container inside it apply in that order (#43)", async () => {
		mount(FIXTURE);
		await frame(
			{ kind: "swap", topic: "drops", html: `<div data-tc-collection="drops.list" data-tc-ver="5"><div data-tc-item="n1"><p>N1</p></div></div>` },
			{ kind: "ops", topic: "drops.list", ver: 6, ops: [up("n9")] },
		);
		expect(itemKeys(containerOf("drops.list"))).toEqual(["n9", "n1"]);
		expect(containerOf("drops.list").getAttribute("data-tc-ver")).toBe("6");
	});

	test("an op then a swap of its region: the swap's state wins and the next op finds the gap", async () => {
		mount(FIXTURE);
		const gaps: number[] = [];
		live.onCollectionGap((_, v) => gaps.push(v));
		await frame(
			{ kind: "ops", topic: "drops.list", ver: 2, ops: [up("n2")] },
			{ kind: "swap", topic: "drops", html: `<div data-tc-collection="drops.list" data-tc-ver="1"><div data-tc-item="n1"><p>N1</p></div></div>` },
		);
		expect(itemKeys(containerOf("drops.list"))).toEqual(["n1"]);
		await frame({ kind: "ops", topic: "drops.list", ver: 3, ops: [up("n3")] });
		expect(gaps).toEqual([1]);
	});

	test("a gap reloads the page by default", async () => {
		mount(FIXTURE);
		const reload = vi.spyOn(window.location, "reload").mockImplementation(() => {});
		await frame({ kind: "ops", topic: "drops.list", ver: 9, ops: [up("n9")] });
		expect(reload).toHaveBeenCalledTimes(1);
		expect(itemKeys(containerOf("drops.list"))).toEqual(["n1"]);
	});

	test("a reset makes the container hold exactly its items", async () => {
		mount(FIXTURE);
		await frame({ kind: "reset", topic: "drops.list", ver: 7, html: `<div data-tc-item="n8"><p>N8</p></div><div data-tc-item="n1"><p>N1b</p></div>` });
		expect(itemKeys(containerOf("drops.list"))).toEqual(["n8", "n1"]);
		expect(text('[data-tc-item="n1"]')).toBe("N1b");
		expect(containerOf("drops.list").getAttribute("data-tc-ver")).toBe("7");
	});

	test("onSwapped fires per surface: the container after ops, the region after a morph", async () => {
		mount(FIXTURE);
		const seen: string[] = [];
		live.onSwapped((el) => {
			seen.push(el.outerHTML.slice(0, 48));
		});
		await frame(
			{ kind: "ops", topic: "drops.list", ver: 2, ops: [up("n7")] },
			{ kind: "swap", topic: "metrics", html: `<span data-tc-cell="ws.count">9</span>` },
		);
		expect(seen[0]).toContain("data-tc-collection");
		expect(seen[1]).toContain("data-tc-region");
	});

	test("a tail container takes new items at the bottom; a head one beside it is untouched", async () => {
		mount(FIXTURE + "\n" + TAIL_FIXTURE);
		await frame(
			{ kind: "ops", topic: "brain.conv", ver: 3, ops: [up("m2"), up("m3")] },
			{ kind: "ops", topic: "drops.list", ver: 2, ops: [up("n2")] },
		);
		expect(itemKeys(containerOf("brain.conv"))).toEqual(["m1", "m2", "m3"]);
		expect(itemKeys(containerOf("drops.list"))).toEqual(["n2", "n1"]);
	});
});

describe("control events", () => {
	test("ready marks the session ready and drains onReady listeners", () => {
		mount(FIXTURE);
		const fired: string[] = [];
		tc.onReady(() => fired.push("queued"));
		expect(tc.isReady()).toBe(false);
		live.apply({ kind: "ready" });
		expect(tc.isReady()).toBe(true);
		expect(fired).toEqual(["queued"]);
		const immediate = vi.fn();
		tc.onReady(immediate);
		expect(immediate).toHaveBeenCalledTimes(1);
	});

	test("resync reloads the page immediately", () => {
		mount(FIXTURE);
		const reload = vi.spyOn(window.location, "reload").mockImplementation(() => {});
		live.apply({ kind: "resync" });
		expect(reload).toHaveBeenCalledTimes(1);
		expect(vi.getTimerCount()).toBe(0);
	});

	test("unknown kinds are frameless; malformed known kinds frame a no-op", async () => {
		const { morphs } = recordMorphs();
		mount(FIXTURE);
		live.apply({ kind: "chat" });
		expect(vi.getTimerCount()).toBe(0);
		live.apply({ kind: "cell" });
		live.apply({ kind: "swap" });
		live.apply({ kind: "ops" });
		live.apply({ kind: "reset", topic: "drops.list" });
		await settle();
		expect(text('[data-tc-cell="ws.count"]')).toBe("0");
		expect(text("[data-tc-island]")).toBe("LIVE DOM");
		expect(morphs).toHaveLength(0);
	});
});

describe("failure containment", () => {
	test("a throwing bound island does not block later cells, ops or swaps", async () => {
		tc.registerBinds({ binds: { "bad-island-contain": [{ topic: "contain.bad", field: "v" }] } });
		mount(`
			<span data-tc-cell="contain.bad">0</span>
			<span data-tc-cell="contain.ok">0</span>
			<bad-island data-tc-island="bad-island-contain">seed</bad-island>
			<div data-tc-region="contain.swap"><p>old</p></div>
			<div data-tc-collection="contain.list" data-tc-ver="0"></div>`);
		const host = document.querySelector('[data-tc-island="bad-island-contain"]')!;
		(host as unknown as { tcBind: () => void }).tcBind = () => {
			throw new Error("island bind throws");
		};

		await frame(
			{ kind: "cell", topic: "contain.bad", v: 1 },
			{ kind: "cell", topic: "contain.ok", v: 7 },
			{ kind: "ops", topic: "contain.list", ver: 1, ops: [up("s1", "ARRIVED")] },
			{ kind: "swap", topic: "contain.swap", html: "<p data-tc-text='new'>NEW</p>" },
		);
		expect(text('[data-tc-cell="contain.ok"]')).toBe("7");
		expect(itemKeys(containerOf("contain.list"))).toEqual(["s1"]);
		expect(regionOf("contain.swap").querySelector('[data-tc-text="new"]')?.textContent).toBe("NEW");

		await frame({ kind: "cell", topic: "contain.bad", v: 2 }, { kind: "cell", topic: "contain.ok", v: 8 });
		expect(text('[data-tc-cell="contain.ok"]')).toBe("8");
		expect(text('[data-tc-cell="contain.bad"]')).toBe("2");
	});

	test("a throwing bind is recorded in tcState, the flush keeps going", async () => {
		tc.registerBinds({ binds: { "bad-island-record": [{ topic: "record.bad", field: "v" }] } });
		mount(`
			<span data-tc-cell="record.bad">0</span>
			<bad-island data-tc-island="bad-island-record">seed</bad-island>`);
		const host = document.querySelector('[data-tc-island="bad-island-record"]')!;
		(host as unknown as { tcBind: () => void }).tcBind = () => {
			throw new Error("recorded failure");
		};
		await frame({ kind: "cell", topic: "record.bad", v: 1 });
		const state = tc.tcState();
		expect(state.phases.live).toBe("failed");
		expect(state.lastErrors.live).toBe("recorded failure");
	});
});
