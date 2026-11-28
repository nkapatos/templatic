// The server sends a page only the cells and regions it uses: those in its markup, those its islands bind, and those
// client code reads through cell(). They ride the stream's c= list with the collections; later markup subscribes.

import { holdsFrom, subscribeMissing } from "./subscribe";
import { bindsOfTag, onWanted, wantedTopics } from "./tc";

const CELL_ATTR = "data-tc-a-";

/** used maps each cell and region topic inside root to -1 (they carry no version). */
export function used(root: ParentNode = document): Map<string, number> {
	const out = new Map<string, number>();
	const each = (sel: string, fn: (el: Element) => void): void => {
		for (const el of root.querySelectorAll(sel)) fn(el);
	};
	each("[data-tc-cell]", (el) => out.set(el.getAttribute("data-tc-cell")!, -1));
	each("[data-tc-region]", (el) => out.set(el.getAttribute("data-tc-region")!, -1));
	each("[data-tc-a]", (el) => {
		for (const name of el.getAttributeNames()) {
			if (name.startsWith(CELL_ATTR)) out.set(el.getAttribute(name)!, -1);
		}
	});
	each("[data-tc-island]", (el) => {
		for (const b of bindsOfTag(el.getAttribute("data-tc-island")!)) out.set(b.topic, -1);
	});
	return out;
}

holdsFrom(() => {
	const out = used();
	for (const topic of wantedTopics()) out.set(topic, -1);
	return out;
});
onWanted((topic) => subscribeMissing(new Map([[topic, -1]])));

/** subscribeUsed subscribes to the cells and regions inside root this connection is not subscribed to yet. */
export function subscribeUsed(root: ParentNode): void {
	subscribeMissing(used(root));
}
