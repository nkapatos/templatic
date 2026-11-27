import { Idiomorph } from "idiomorph";

import { applyOps, applyReset, forget, subscribeShown, type GapHandler, type Op } from "./collections";
import { applyDataOps, applyDataReset, deliverData } from "./data";
import { catchUp } from "./subscribe";
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
	/** ver is the collection version after an ops batch, or of a reset. */
	ver?: number;
	/** ops carry html for a rendered collection, v for a data collection. */
	ops?: Op[];
	/** items is a data collection reset's entries. */
	items?: unknown;
	what?: string;
	css?: Record<string, string>;
	message?: string;
}

/** SwappedCallback is told which region was swapped or which collection container changed. */
export type SwappedCallback = (root: Element) => void;

// Arrival order, so a swap and a later op on a container inside it apply in that order (#43).
let pending: WireEvent[] = [];
let swappedCallback: SwappedCallback | null = null;
let flushScheduled = false;

// A container that missed ops, such as one a racing region swap rolled back, asks for them from the version it holds.
let gapHandler: GapHandler = catchUp;

/** onSwapped sets the callback run after a region swap or a collection change. */
export function onSwapped(fn: SwappedCallback): void {
	swappedCallback = fn;
}

/** onCollectionGap sets what runs when a collection container missed ops. */
export function onCollectionGap(fn: GapHandler): void {
	gapHandler = fn;
}

/** apply queues a wire event; cell, swap, ops and reset are flushed per animation frame, in arrival order. */
export function apply(ev: WireEvent): void {
	switch (ev.kind) {
		case "cell":
			if (!ev.topic) return;
			break;
		case "swap":
			if (!ev.topic || typeof ev.html !== "string") return;
			break;
		case "ops":
			if (!ev.topic || typeof ev.ver !== "number" || !Array.isArray(ev.ops)) return;
			break;
		case "reset":
			if (!ev.topic || typeof ev.ver !== "number" || (typeof ev.html !== "string" && !Array.isArray(ev.items))) return;
			break;
		case "ready":
			markReady();
			return;
		case "resync":
			recordLive("resync");
			abandonPending();
			location.reload();
			return;
		case "dev":
			applyDev(ev as DevFrame);
			return;
		default:
			return;
	}
	pending.push(ev);
	scheduleFlush();
}

function scheduleFlush(): void {
	if (flushScheduled) return;
	flushScheduled = true;
	requestAnimationFrame(() => {
		flushScheduled = false;
		flush();
	});
}

// A throwing tcBind, morph or hook must neither block later events nor stay queued (it would re-throw every frame).
function flush(): void {
	const batch = pending;
	pending = [];
	// cells and swaps are latest-wins: one superseded by a later one of its kind and topic is skipped
	const last = new Map<string, number>();
	batch.forEach((ev, i) => {
		if (ev.kind === "cell" || ev.kind === "swap") last.set(`${ev.kind}\0${ev.topic}`, i);
	});
	batch.forEach((ev, i) => {
		const topic = ev.topic!;
		if ((ev.kind === "cell" || ev.kind === "swap") && last.get(`${ev.kind}\0${topic}`) !== i) return;
		try {
			switch (ev.kind) {
				case "cell":
					deliverCell(topic, ev.v);
					break;
				case "swap":
					morph(topic, ev.html!);
					break;
				case "ops":
					for (const c of applyOps(topic, ev.ver!, ev.ops!, gapHandler)) swappedCallback?.(c);
					applyDataOps(topic, ev.ver!, ev.ops!);
					break;
				case "reset":
					if (typeof ev.html === "string") {
						for (const c of applyReset(topic, ev.ver!, ev.html)) swappedCallback?.(c);
					} else {
						applyDataReset(topic, ev.ver!, ev.items);
					}
					break;
			}
		} catch (err) {
			console.error(`tc: ${ev.kind} failed for ${topic} — later updates still apply; window.tcState() carries the record`, err);
			tcPhase("live", err);
		}
	});
	// once per frame, however many batches a data collection received in it
	try {
		deliverData();
	} catch (err) {
		console.error("tc: delivering collection items failed", err);
		tcPhase("live", err);
	}
}

function deliverCell(topic: string, value: unknown): void {
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

function morph(topic: string, html: string): void {
	const region = document.querySelector(`[data-tc-region="${topic}"]`);
	if (!region) return;
	emit("swap:before", { topic, el: region });
	// idiomorph soft-matches a same-position host and would morph the payload's placeholder
	// into the island's live DOM, destroying its state; remove hosts so the payload's land fresh.
	for (const host of region.querySelectorAll("[data-tc-island]")) {
		host.replaceWith(document.createComment("tc-island"));
	}
	// innerHTML keeps the region marker; outerHTML would replace it and the next swap would find nothing.
	Idiomorph.morph(region, html, { morphStyle: "innerHTML", callbacks: morphCallbacks });
	forget(region);
	subscribeShown(region);
	scanHooks(region);
	swappedCallback?.(region);
	emit("swap:after", { topic, el: region });
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
