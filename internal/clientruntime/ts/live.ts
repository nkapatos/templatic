import { Idiomorph } from "idiomorph";

import { abandonPending } from "./commands";
import { applyDev, type DevFrame } from "./dev";
import { morphCallbacks, scanHooks } from "./hooks";
import { bindsFor, emit, markReady, recordLive, setLatestCell, tcPhase } from "./tc";

/** WireEvent is one server event frame. */
export interface WireEvent {
	kind: string;
	topic?: string;
	v?: unknown;
	html?: string;
	items?: StreamItem[];
	what?: string;
	css?: Record<string, string>;
	message?: string;
}

/** StreamItem is one keyed item of an append event; keys obey the topic grammar, so they are safe in selectors. */
export interface StreamItem {
	k: string;
	html: string;
}

/** SwappedCallback is told which region was swapped or appended to. */
export type SwappedCallback = (region: Element) => void;

const pendingCells = new Map<string, unknown>();
const pendingSwaps = new Map<string, string>();
const pendingAppends = new Map<string, StreamItem[]>();
let swappedCallback: SwappedCallback | null = null;
let morphing: Promise<void> = Promise.resolve();
let flushScheduled = false;

/** onSwapped sets the callback run after a region swap or append. */
export function onSwapped(fn: SwappedCallback): void {
	swappedCallback = fn;
}

/** apply queues a wire event; cell, swap and append are flushed per animation frame. */
export function apply(ev: WireEvent): void {
	switch (ev.kind) {
		case "cell":
			if (ev.topic) pendingCells.set(ev.topic, ev.v);
			scheduleFlush();
			break;
		case "swap":
			if (ev.topic && typeof ev.html === "string") pendingSwaps.set(ev.topic, ev.html);
			scheduleFlush();
			break;
		case "append":
			// items arrive oldest-first
			if (ev.topic && Array.isArray(ev.items)) {
				const list = pendingAppends.get(ev.topic) ?? [];
				pendingAppends.set(ev.topic, list.concat(ev.items));
			}
			scheduleFlush();
			break;
		case "ready":
			markReady();
			break;
		case "resync":
			recordLive("resync");
			abandonPending();
			location.reload();
			break;
		case "dev":
			applyDev(ev as DevFrame);
			break;
	}
}

function scheduleFlush(): void {
	if (flushScheduled) return;
	flushScheduled = true;
	requestAnimationFrame(() => {
		flushScheduled = false;
		void flush();
	});
}

async function flush(): Promise<void> {
	// A throwing tcBind must neither block later topics nor leave the map uncleared (it would re-throw every frame).
	if (pendingCells.size > 0) {
		const cells = [...pendingCells];
		pendingCells.clear();
		for (const [topic, value] of cells) {
			try {
				// the latch re-seeds island subscribers whose chunk loads later
				setLatestCell(topic, value);
				for (const el of document.querySelectorAll(`[data-tc-cell="${topic}"]`)) {
					el.textContent = renderValue(value);
				}
				for (const el of document.querySelectorAll("[data-tc-a]")) {
					applyCellAttrs(el, topic, value);
				}
				deliverBinds(topic, value);
				emit("cell", { topic, value });
			} catch (err) {
				console.error(`tc: cell delivery failed for ${topic} — later updates still apply`, err);
				tcPhase("live", err);
			}
		}
	}
	if (pendingAppends.size > 0) {
		const appends = [...pendingAppends];
		pendingAppends.clear();
		for (const [topic, items] of appends) {
			try {
				applyAppends(topic, items);
			} catch (err) {
				console.error(`tc: append failed for ${topic} — later updates still apply`, err);
				tcPhase("live", err);
			}
		}
	}
	for (const topic of [...pendingSwaps.keys()]) {
		if (document.querySelector(`[data-tc-region="${topic}"]`)) continue;
		pendingSwaps.delete(topic);
	}
	if (pendingSwaps.size > 0) {
		// Serialized so mutation records cannot interleave; a rejected morph must not poison later swaps.
		morphing = morphing
			.then(() => morphAll())
			.catch((err: unknown) => {
				console.error("tc: swap morph failed — later swaps still apply; window.tcState() carries the record", err);
				tcPhase("live", err);
			});
		await morphing;
	}
}

// tcBind is duck-typed: only bound islands define it, and an un-upgraded host is re-seeded from the latch on load.
function deliverBinds(topic: string, value: unknown): void {
	for (const b of bindsFor(topic)) {
		for (const host of document.querySelectorAll(`[data-tc-island="${b.tag}"]`)) {
			const bound = host as unknown as { tcBind?: (field: string, v: unknown) => void };
			bound.tcBind?.(b.field, value);
		}
	}
}

// Keys already present are skipped (double delivery, catch-up); data-tc-dir="tail" appends at the tail, else the head.
function applyAppends(topic: string, items: StreamItem[]): void {
	const container = document.querySelector(`[data-tc-stream="${topic}"]`);
	if (!container) return;
	const tail = container.getAttribute("data-tc-dir") === "tail";
	const added: Array<{ key: string; el: Element }> = [];
	for (const item of items) {
		if (container.querySelector(`[data-tc-item="${item.k}"]`)) continue;
		const tpl = document.createElement("template");
		tpl.innerHTML = item.html;
		const node = tpl.content.firstElementChild;
		if (node) {
			if (tail) {
				container.append(node);
			} else {
				container.prepend(node);
			}
			added.push({ key: item.k, el: node });
		}
	}
	for (const a of added) scanHooks(a.el);
	swappedCallback?.(container);
	for (const a of added) emit("append", { topic, key: a.key, el: a.el });
}

function morphAll(): void {
	for (const [topic, html] of pendingSwaps) {
		pendingSwaps.delete(topic);
		const region = document.querySelector(`[data-tc-region="${topic}"]`);
		if (!region) continue;
		emit("swap:before", { topic, el: region });
		// idiomorph soft-matches a same-position host and would morph the payload's placeholder
		// into the island's live DOM, destroying its state; remove hosts so the payload's land fresh.
		for (const host of region.querySelectorAll("[data-tc-island]")) {
			host.replaceWith(document.createComment("tc-island"));
		}
		// innerHTML keeps the region marker; outerHTML would replace it and the next swap would find nothing.
		Idiomorph.morph(region, html, { morphStyle: "innerHTML", callbacks: morphCallbacks });
		scanHooks(region);
		swappedCallback?.(region);
		emit("swap:after", { topic, el: region });
	}
}

// The Go allowlist is re-checked so hand-written markup cannot turn a cell into a URL, style or script sink.
const CELL_ATTR = "data-tc-a-";
const BINDABLE = /^(class|value|data-(?!tc-)[a-z0-9_.-]+|aria-[a-z0-9_.-]+)$/;

/** AttrTarget is the element surface applyCellAttrs touches. */
export interface AttrTarget {
	getAttributeNames(): string[];
	getAttribute(name: string): string | null;
	setAttribute(name: string, value: string): void;
	removeAttribute(name: string): void;
}

/** applyCellAttrs writes value into el's attributes bound to topic (empty removes) and returns the count written. */
export function applyCellAttrs(el: AttrTarget, topic: string, value: unknown): number {
	let n = 0;
	for (const name of el.getAttributeNames()) {
		if (!name.startsWith(CELL_ATTR) || el.getAttribute(name) !== topic) continue;
		const attr = name.slice(CELL_ATTR.length);
		if (!BINDABLE.test(attr)) {
			console.warn(`tc: refusing cell binding to attribute ${attr}`);
			continue;
		}
		const text = renderValue(value);
		if (text === "") el.removeAttribute(attr);
		else el.setAttribute(attr, text);
		if (attr === "value" && "value" in el) (el as unknown as { value: string }).value = text;
		n++;
	}
	return n;
}

// Must match the server's first-paint render rule character for character, or text changes on first publish.
function renderValue(v: unknown): string {
	if (v == null) return "";
	if (typeof v === "string") return v;
	if (typeof v === "number" || typeof v === "boolean") return String(v);
	return JSON.stringify(v);
}
