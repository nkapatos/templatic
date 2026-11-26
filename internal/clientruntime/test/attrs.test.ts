import { beforeEach, expect, test, vi } from "vitest";

import { frame } from "./support/frame";

let live: typeof import("../ts/live");

beforeEach(async () => {
	vi.useFakeTimers();
	vi.resetModules();
	live = await import("../ts/live");
});

function el(html: string): Element {
	document.body.innerHTML = html;
	return document.body.firstElementChild!;
}

test("cell attr writes, re-renders and removes on empty", () => {
	const orb = el('<div id="orb" data-tc-a="" data-tc-a-data-state="orb.state" data-state="idle" class="orb"></div>');
	expect(live.applyCellAttrs(orb, "orb.state", "analyzing")).toBe(1);
	expect(orb.getAttribute("data-state")).toBe("analyzing");
	live.applyCellAttrs(orb, "orb.state", 3);
	expect(orb.getAttribute("data-state")).toBe("3");
	live.applyCellAttrs(orb, "orb.state", null);
	expect(orb.hasAttribute("data-state")).toBe(false);
	live.applyCellAttrs(orb, "orb.state", "x");
	live.applyCellAttrs(orb, "orb.state", "");
	expect(orb.hasAttribute("data-state")).toBe(false);
	expect(orb.getAttribute("class")).toBe("orb");
});

test("cell attr binds several attributes and only its own topic", () => {
	const b = el('<b id="x" data-tc-a="" data-tc-a-aria-busy="a.busy" data-tc-a-class="a.cls" class="old"></b>');
	expect(live.applyCellAttrs(b, "other.topic", "nope")).toBe(0);
	live.applyCellAttrs(b, "a.busy", true);
	live.applyCellAttrs(b, "a.cls", "pill on");
	expect([b.getAttribute("aria-busy"), b.getAttribute("class")]).toEqual(["true", "pill on"]);
});

test("cell attr refuses non-allowlisted attributes", () => {
	const a = el('<a id="l" data-tc-a="" data-tc-a-href="t" data-tc-a-onclick="t" data-tc-a-style="t" data-tc-a-data-tc-cell="t" href="/safe"></a>');
	vi.spyOn(console, "warn").mockImplementation(() => {});
	expect(live.applyCellAttrs(a, "t", "javascript:alert(1)")).toBe(0);
	expect(a.getAttribute("href")).toBe("/safe");
	expect(a.hasAttribute("onclick")).toBe(false);
	expect(a.hasAttribute("style")).toBe(false);
});

test("cell events reach attribute bindings through the wire", async () => {
	document.body.innerHTML = '<div data-tc-a="" data-tc-a-data-state="orb.state"></div><span data-tc-cell="orb.state"></span>';
	await frame({ kind: "cell", topic: "orb.state", v: "ready" });
	expect(document.querySelector("[data-tc-a]")?.getAttribute("data-state")).toBe("ready");
	expect(document.querySelector("[data-tc-cell]")?.textContent).toBe("ready");
});
