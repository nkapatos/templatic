import { beforeEach, describe, expect, test, vi } from "vitest";

let col: typeof import("../ts/collections");
let hooks: typeof import("../ts/hooks");

beforeEach(async () => {
	vi.resetModules();
	col = await import("../ts/collections");
	hooks = await import("../ts/hooks");
});

const u = (k: string, html: string) => ({ op: "u" as const, k, html });
const keys = (sel = "[data-tc-item]") => [...document.querySelectorAll<HTMLElement>(sel)].map((e) => e.dataset.tcItem).join(",");
const noGap = () => {
	throw new Error("unexpected gap");
};

describe("applyOps", () => {
	test("inserts at the head, updates in place and removes, on table rows", () => {
		document.body.innerHTML = `<table><tbody data-tc-collection="rows" data-tc-ver="0"><tr data-tc-item="a"><td>A</td></tr></tbody></table>`;
		col.applyOps("rows", 3, [u("b", `<tr data-tc-item="b"><td>B</td></tr>`), u("a", `<tr data-tc-item="a"><td>A2</td></tr>`), u("c", `<tr data-tc-item="c"><td>C</td></tr>`)], noGap);
		col.applyOps("rows", 4, [{ op: "d", k: "b" }], noGap);
		const tbody = document.querySelector("tbody")!;
		expect([...tbody.children].map((r) => `${r.tagName}:${r.textContent}`)).toEqual(["TR:C", "TR:A2"]);
		expect(tbody.getAttribute("data-tc-ver")).toBe("4");
	});

	test("tail inserts at the bottom and the limit drops the oldest", () => {
		document.body.innerHTML = `<div data-tc-collection="t" data-tc-ver="0" data-tc-dir="tail" data-tc-limit="3"></div>`;
		col.applyOps("t", 5, [1, 2, 3, 4, 5].map((n) => u(`m${n}`, `<p data-tc-item="m${n}">${n}</p>`)), noGap);
		expect(document.querySelector("[data-tc-collection]")!.textContent).toBe("345");
	});

	test("a head limit drops from the bottom", () => {
		document.body.innerHTML = `<ul data-tc-collection="h" data-tc-ver="0" data-tc-limit="2"></ul>`;
		col.applyOps("h", 3, ["a", "b", "c"].map((k) => u(k, `<li data-tc-item="${k}">${k}</li>`)), noGap);
		expect(keys()).toBe("c,b");
	});

	test("skips ops the container holds and reports a gap without applying", () => {
		document.body.innerHTML = `<div data-tc-collection="v" data-tc-ver="3"></div>`;
		const gaps: number[] = [];
		const item = (k: string) => u(k, `<i data-tc-item="${k}">${k}</i>`);
		col.applyOps("v", 5, [item("v2"), item("v3"), item("v4"), item("v5")], (_, v) => gaps.push(v));
		col.applyOps("v", 9, [item("v8"), item("v9")], (_, v) => gaps.push(v));
		expect(keys()).toBe("v5,v4");
		expect(gaps).toEqual([5]);
		expect(document.querySelector("[data-tc-collection]")!.getAttribute("data-tc-ver")).toBe("5");
	});

	test("applies to every container of the topic", () => {
		document.body.innerHTML = `<ol data-tc-collection="d" data-tc-ver="0"></ol><ol data-tc-collection="d" data-tc-ver="0"></ol>`;
		col.applyOps("d", 1, [u("a", `<li data-tc-item="a">a</li>`)], noGap);
		expect(document.querySelectorAll('[data-tc-item="a"]')).toHaveLength(2);
	});

	test("an update keeps the focused field's typed value", () => {
		document.body.innerHTML = `<div data-tc-collection="f" data-tc-ver="0"><div data-tc-item="a"><span>v1</span><input value=""></div></div>`;
		const input = document.querySelector("input")!;
		input.focus();
		input.value = "typed";
		col.applyOps("f", 1, [u("a", `<div data-tc-item="a"><span>v2</span><input value=""></div>`)], noGap);
		expect(document.querySelector("input")).toBe(input);
		expect(input.value).toBe("typed");
		expect(document.querySelector("span")!.textContent).toBe("v2");
	});

	test("an update re-renders a focused button, keeping its focus", () => {
		document.body.innerHTML = `<div data-tc-collection="f" data-tc-ver="0"><div data-tc-item="a"><button>♥ 0</button></div></div>`;
		const button = document.querySelector("button")!;
		button.focus();
		col.applyOps("f", 1, [u("a", `<div data-tc-item="a"><button>♥ 1</button></div>`)], noGap);
		expect(document.querySelector("button")).toBe(button);
		expect(button.textContent).toBe("♥ 1");
		expect(document.activeElement).toBe(button);
	});

	test("hooks mount on insert and are destroyed on remove and trim", () => {
		document.body.innerHTML = `<div data-tc-collection="h" data-tc-ver="0" data-tc-limit="2"></div>`;
		const log: string[] = [];
		hooks.defineHook("probe", { mounted: () => void log.push("m"), destroyed: () => void log.push("d") });
		const item = (k: string) => u(k, `<div data-tc-item="${k}"><b data-tc-hook="probe"></b></div>`);
		col.applyOps("h", 3, [item("a"), item("b"), item("c")], noGap);
		col.applyOps("h", 4, [{ op: "d", k: "c" }], noGap);
		expect(keys()).toBe("b");
		expect(log.filter((x) => x === "m")).toHaveLength(3);
		expect(log.filter((x) => x === "d")).toHaveLength(2);
	});

	test("forget makes a container rewritten by a morph found again", () => {
		document.body.innerHTML = `<div data-tc-region="r"><div data-tc-collection="s" data-tc-ver="1"><p data-tc-item="a">a</p></div></div>`;
		const c = document.querySelector("[data-tc-collection]")!;
		col.applyOps("s", 2, [{ op: "d", k: "zz" }], noGap); // builds the index
		c.innerHTML = `<p data-tc-item="b">b</p>`;
		col.forget(document.querySelector("[data-tc-region]")!);
		col.applyOps("s", 3, [{ op: "d", k: "b" }], noGap);
		expect(keys()).toBe("");
	});
});

describe("applyReset", () => {
	test("holds exactly the new items, keeping a reordered focused item in place", () => {
		document.body.innerHTML = `<div data-tc-collection="z" data-tc-ver="1"><div data-tc-item="a"><input value=""></div><div data-tc-item="b">b</div></div>`;
		const input = document.querySelector("input")!;
		input.focus();
		input.value = "typed";
		col.applyReset("z", 9, `<div data-tc-item="c">c</div><div data-tc-item="a"><input value=""></div>`);
		expect(keys()).toBe("c,a");
		expect(document.querySelector("input")).toBe(input);
		expect(input.value).toBe("typed");
		expect(document.activeElement).toBe(input);
		expect(document.querySelector("[data-tc-collection]")!.getAttribute("data-tc-ver")).toBe("9");
	});
});
