import { expect, test } from "vitest";

import { swapTargetMissing, swapsAttr } from "../ts/actions";

function trigger(attrs: Record<string, string>): Element {
	const el = document.createElement("button");
	for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
	return el;
}

const WITH_REGION = `<div data-tc-region="workspace.surface"><p>surface</p></div>`;
const WITHOUT_REGION = `<div data-tc-region="other.surface"><p>other</p></div>`;

test("a declared swap target that is absent here means: navigate", () => {
	document.body.innerHTML = WITHOUT_REGION;
	expect(swapTargetMissing(trigger({ [swapsAttr]: "workspace.surface" }), document)).toBe(true);
});

test("a declared swap target that is present here means: run the command", () => {
	document.body.innerHTML = WITH_REGION;
	expect(swapTargetMissing(trigger({ [swapsAttr]: "workspace.surface" }), document)).toBe(false);
});

test("an undeclared trigger keeps the command path (no regression)", () => {
	document.body.innerHTML = WITHOUT_REGION;
	expect(swapTargetMissing(trigger({}), document)).toBe(false);
});
