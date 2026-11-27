import { isSnapshot, seed } from "./data";
import { bindsOfTag } from "./tc";

export function readProps<T>(el: Element): T | null {
	const script = el.querySelector(
		":scope > script[type='application/tc-props']",
	);
	if (!script?.textContent?.trim()) return null;
	try {
		const props = JSON.parse(script.textContent) as Record<string, unknown> | null;
		unwrapSnapshots(el, props);
		return props as T;
	} catch (err) {
		console.warn("tc: bad props payload", err);
		return null;
	}
}

export function clearPropsScript(el: Element): void {
	const script = el.querySelector(
		":scope > script[type='application/tc-props']",
	);
	script?.remove();
}

// A field bound to a data collection arrives as a live.Snapshot: the page holds the collection from it, and the
// island gets the items.
function unwrapSnapshots(el: Element, props: Record<string, unknown> | null): void {
	if (!props || typeof props !== "object") return;
	const tag = el.getAttribute("data-tc-island") ?? el.localName;
	for (const b of bindsOfTag(tag)) {
		const v = props[b.field];
		if (isSnapshot(v)) props[b.field] = seed(b.topic, v);
	}
}
