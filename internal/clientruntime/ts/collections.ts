import { Idiomorph } from "idiomorph";

import { destroyUnder, morphCallbacks, scanHooks } from "./hooks";
import { emit } from "./tc";

/** Op is one change to a collection: an upsert of key k ("u") or its removal ("d"). */
export interface Op {
	op: "u" | "d";
	k: string;
	html?: string;
}

/** GapHandler is told a container holds version ver of topic and missed the ops after it. */
export type GapHandler = (topic: string, ver: number) => void;

const VER = "data-tc-ver";
const ITEM = "data-tc-item";

// Per container, key → item element. A hit is verified; a miss trusts the index, so forget must run after anything
// else rewrites a container's children (a region swap).
const indexes = new WeakMap<Element, Map<string, Element>>();

// The focused field keeps what the user typed; the server's value lands once it loses focus.
const morphOptions = { callbacks: morphCallbacks, ignoreActiveValue: true };

/** forget drops the item indexes of containers under root (root included), after a morph rewrote them. */
export function forget(root: Element): void {
	indexes.delete(root);
	for (const c of root.querySelectorAll("[data-tc-collection]")) indexes.delete(c);
}

/** containers returns every container of topic on the page. */
export function containers(topic: string): Element[] {
	return [...document.querySelectorAll(`[data-tc-collection="${topic}"]`)];
}

/** version returns the version a container holds, -1 when it has none. */
export function version(container: Element): number {
	const v = container.getAttribute(VER);
	if (v === null || v === "") return -1;
	const n = Number(v);
	return Number.isInteger(n) ? n : -1;
}

function rebuild(container: Element): Map<string, Element> {
	const m = new Map<string, Element>();
	for (const el of container.children) {
		const k = el.getAttribute(ITEM);
		if (k !== null) m.set(k, el);
	}
	indexes.set(container, m);
	return m;
}

function lookup(container: Element, key: string): Element | undefined {
	const m = indexes.get(container) ?? rebuild(container);
	const el = m.get(key);
	if (!el) return undefined;
	if (el.parentElement === container && el.getAttribute(ITEM) === key) return el;
	return rebuild(container).get(key);
}

function parse(html: string): Element | null {
	// a template parses any item root, <tr> and <li> included, outside its eventual parent
	const tpl = document.createElement("template");
	tpl.innerHTML = html;
	return tpl.content.firstElementChild;
}

// Islands are units: a payload's host lands fresh instead of being morphed into the live one.
function dropIslands(root: Element): void {
	for (const host of root.querySelectorAll("[data-tc-island]")) {
		host.replaceWith(document.createComment("tc-island"));
	}
}

function remove(topic: string, container: Element, el: Element, key: string): void {
	destroyUnder(el);
	el.remove();
	indexes.get(container)?.delete(key);
	emit("item", { topic, key, el, change: "remove" });
}

// Every element child of a container is an item, so trimming touches only the excess at the far end.
function trim(topic: string, container: Element): void {
	const limit = Number(container.getAttribute("data-tc-limit") ?? "");
	if (!Number.isInteger(limit) || limit <= 0) return;
	const tail = container.getAttribute("data-tc-dir") === "tail";
	// the far end is the oldest: the bottom for head inserts, the top for tail inserts
	for (let excess = container.childElementCount - limit; excess > 0; excess--) {
		const el = (tail ? container.firstElementChild : container.lastElementChild)!;
		remove(topic, container, el, el.getAttribute(ITEM) ?? "");
	}
}

function upsert(topic: string, container: Element, op: Op): void {
	if (typeof op.html !== "string") return;
	const fresh = parse(op.html);
	if (!fresh || fresh.getAttribute(ITEM) !== op.k) return;
	const old = lookup(container, op.k);
	if (old) {
		dropIslands(old);
		// a parsed node, not the string: idiomorph's own parse would drop a <tr> or <li> root
		Idiomorph.morph(old, fresh, { ...morphOptions, morphStyle: "outerHTML" });
		const el = lookup(container, op.k);
		if (!el) return;
		scanHooks(el);
		emit("item", { topic, key: op.k, el, change: "update" });
		return;
	}
	const el = fresh;
	if (container.getAttribute("data-tc-dir") === "tail") container.append(el);
	else container.prepend(el);
	indexes.get(container)?.set(op.k, el);
	scanHooks(el);
	emit("item", { topic, key: op.k, el, change: "insert" });
}

/**
 * applyOps applies a batch ending at version ver to every container of topic. Ops a container already holds are
 * skipped; a batch that starts past a container's next version is not applied there and onGap is told instead.
 * It returns the containers it changed.
 */
export function applyOps(topic: string, ver: number, ops: Op[], onGap: GapHandler): Element[] {
	const first = ver - ops.length + 1;
	const changed: Element[] = [];
	for (const container of containers(topic)) {
		const local = version(container);
		if (local >= 0 && first > local + 1) {
			onGap(topic, local);
			continue;
		}
		const fresh = local < 0 ? ops : ops.slice(Math.max(0, local + 1 - first));
		if (fresh.length === 0) continue;
		for (const op of fresh) {
			if (op.op === "d") {
				const el = lookup(container, op.k);
				if (el) remove(topic, container, el, op.k);
			} else {
				upsert(topic, container, op);
			}
		}
		trim(topic, container);
		container.setAttribute(VER, String(ver));
		changed.push(container);
	}
	return changed;
}

/**
 * applyReset makes every container of topic hold exactly the items in html, at version ver. Items it already holds
 * are morphed in place and moved only when out of order, so a focused item keeps its focus and typed value. It
 * returns the containers it changed.
 */
export function applyReset(topic: string, ver: number, html: string): Element[] {
	const changed = containers(topic);
	for (const container of changed) {
		const tpl = document.createElement("template");
		tpl.innerHTML = html;
		const next = [...tpl.content.children].filter((el) => el.hasAttribute(ITEM));
		const wanted = new Set(next.map((el) => el.getAttribute(ITEM)!));
		for (const el of [...container.children]) {
			const k = el.getAttribute(ITEM);
			if (k !== null && !wanted.has(k)) remove(topic, container, el, k);
		}
		const held = rebuild(container);
		let cursor: Element | null = container.firstElementChild;
		for (const fresh of next) {
			const k = fresh.getAttribute(ITEM)!;
			let el = held.get(k);
			if (el) {
				const wasCursor = el === cursor;
				const before = el.previousSibling;
				dropIslands(el);
				Idiomorph.morph(el, fresh, { ...morphOptions, morphStyle: "outerHTML" });
				// a morph that changes the root's tag replaces the node in place
				if (!el.isConnected) el = (before ? before.nextSibling : container.firstChild) as Element;
				if (wasCursor) cursor = el;
			} else {
				el = fresh;
			}
			if (el === cursor) {
				cursor = cursor.nextElementSibling;
			} else {
				container.insertBefore(el, cursor);
			}
		}
		rebuild(container);
		container.setAttribute(VER, String(ver));
		trim(topic, container);
		scanHooks(container);
		emit("item", { topic, key: "", el: container, change: "reset" });
	}
	return changed;
}
