import { catchUp, holdsFrom, subscribeMissing } from "./subscribe";
import { bindsFor, type Unsubscribe } from "./tc";

/** DataOp is one change to a data collection: an upsert of key k with its item v ("u"), or its removal ("d"). */
export interface DataOp<T = unknown> {
	op: "u" | "d";
	k: string;
	v?: T;
}

/** Collection is the client view of a data collection topic: its items in display order, kept current by the server. */
export interface Collection<T> {
	/** items returns the items in display order; the array is replaced, never changed, when the collection changes. */
	items(): readonly T[];
	/** get returns the item under key. */
	get(key: string): T | undefined;
	/**
	 * subscribe calls fn with the items, if the page holds the collection, then once per frame in which it changed,
	 * with that frame's ops; ops is null when the whole set was replaced.
	 */
	subscribe(fn: (items: readonly T[], ops: readonly DataOp<T>[] | null) => void): Unsubscribe;
}

interface Entry {
	k: string;
	v: unknown;
}

// The wire form of live.Snapshot, and of a data collection's reset.
interface SnapshotWire {
	tc: "snapshot";
	ver: number;
	tail?: boolean;
	limit?: number;
	items: Entry[];
}

type Listener = (items: readonly unknown[], ops: readonly DataOp[] | null) => void;

class Store {
	ver: number;
	tail: boolean;
	limit: number;
	keys: string[] = []; // display order
	values = new Map<string, unknown>();
	private cached: unknown[] | null = null;
	listeners = new Set<Listener>();
	// ops since the last delivery, which is the end of the frame that applied them; null after a reset, since a
	// listener then needs the whole set
	pending: DataOp[] | null = [];
	dirty = false;

	constructor(s: SnapshotWire) {
		this.ver = s.ver;
		this.tail = s.tail === true;
		this.limit = Number.isInteger(s.limit) && s.limit! > 0 ? s.limit! : 0;
		this.fill(s.items);
	}

	fill(items: Entry[]): void {
		this.keys = [];
		this.values.clear();
		for (const e of items) {
			if (this.values.has(e.k)) continue;
			this.keys.push(e.k);
			this.values.set(e.k, e.v);
		}
		this.cached = null;
	}

	items(): unknown[] {
		this.cached ??= this.keys.map((k) => this.values.get(k));
		return this.cached;
	}

	apply(op: DataOp): void {
		if (op.op === "d") {
			if (!this.values.delete(op.k)) return;
			this.keys.splice(this.keys.indexOf(op.k), 1);
		} else {
			if (!this.values.has(op.k)) {
				if (this.tail) this.keys.push(op.k);
				else this.keys.unshift(op.k);
			}
			this.values.set(op.k, op.v);
		}
		this.cached = null;
		this.pending?.push(op);
		this.dirty = true;
	}

	// The far end holds the oldest, as on the server: the top for tail inserts, the bottom for head inserts.
	trim(): void {
		const excess = this.limit > 0 ? this.keys.length - this.limit : 0;
		if (excess <= 0) return;
		const dropped = this.tail ? this.keys.splice(0, excess) : this.keys.splice(this.keys.length - excess);
		for (const k of dropped) this.values.delete(k);
		this.cached = null;
	}
}

const stores = new Map<string, Store>();
// collection() subscribers whose topic no bound island has seeded yet
const waiting = new Map<Listener, { topic: string; attach: (s: Store) => void }>();

holdsFrom(() => new Map([...stores].map(([topic, s]) => [topic, s.ver])));

/** isSnapshot reports whether v is a live.Snapshot's wire form. */
export function isSnapshot(v: unknown): v is SnapshotWire {
	if (typeof v !== "object" || v === null) return false;
	const s = v as Partial<SnapshotWire>;
	return s.tc === "snapshot" && typeof s.ver === "number" && Array.isArray(s.items);
}

/**
 * seed makes the page hold topic from a snapshot in an island's props and returns the items for the bound field. A
 * store already at the snapshot's version or later is kept: the page may have received ops since it rendered.
 */
export function seed(topic: string, snap: SnapshotWire): unknown[] {
	let store = stores.get(topic);
	if (!store) {
		store = new Store(snap);
		stores.set(topic, store);
		for (const [fn, w] of waiting) {
			if (w.topic !== topic) continue;
			waiting.delete(fn);
			w.attach(store);
		}
	} else if (snap.ver > store.ver) {
		store.ver = snap.ver;
		store.fill(snap.items);
		store.pending = null;
		store.dirty = true;
	}
	subscribeMissing(new Map([[topic, store.ver]]));
	return store.items();
}

/**
 * applyDataOps applies a batch ending at version ver to topic's store, skipping ops it already holds; a batch that
 * starts past its next version is not applied, and the page catches up instead. A topic the page does not hold is
 * ignored: its ops are for rendered containers.
 */
export function applyDataOps(topic: string, ver: number, ops: DataOp[]): void {
	const store = stores.get(topic);
	if (!store) return;
	const first = ver - ops.length + 1;
	if (first > store.ver + 1) {
		catchUp(topic, store.ver);
		return;
	}
	for (const op of ops.slice(Math.max(0, store.ver + 1 - first))) store.apply(op);
	store.trim();
	store.ver = Math.max(store.ver, ver);
}

/** applyDataReset replaces topic's items with a reset's, at version ver. */
export function applyDataReset(topic: string, ver: number, items: unknown): void {
	const store = stores.get(topic);
	if (!store || !Array.isArray(items)) return;
	store.fill(items as Entry[]);
	store.trim();
	store.ver = ver;
	store.pending = null;
	store.dirty = true;
}

/** deliverData hands each changed store's items to its bound island fields and listeners, once per frame. */
export function deliverData(): void {
	for (const [topic, store] of stores) {
		if (!store.dirty) continue;
		const ops = store.pending;
		store.dirty = false;
		store.pending = [];
		const items = store.items();
		for (const b of bindsFor(topic)) {
			for (const host of document.querySelectorAll(`[data-tc-island="${b.tag}"]`)) {
				const bound = host as unknown as { tcBind?: (field: string, v: unknown) => void };
				bound.tcBind?.(b.field, items);
			}
		}
		for (const fn of store.listeners) {
			try {
				fn(items, ops);
			} catch (err) {
				console.error("tc: collection subscriber failed", err);
			}
		}
	}
}

/** collection returns the client view of a data collection topic; it holds items once an island bound to it loads. */
export function collection<T = unknown>(topic: string): Collection<T> {
	return {
		items: () => (stores.get(topic)?.items() ?? []) as readonly T[],
		get: (key) => stores.get(topic)?.values.get(key) as T | undefined,
		subscribe(fn) {
			const listener = fn as Listener;
			const attach = (s: Store): void => {
				s.listeners.add(listener);
				try {
					listener(s.items(), null);
				} catch (err) {
					console.error("tc: collection subscriber failed", err);
				}
			};
			const store = stores.get(topic);
			if (store) {
				attach(store);
				return () => store.listeners.delete(listener);
			}
			// the store appears when a bound island seeds it
			waiting.set(listener, { topic, attach });
			return () => {
				waiting.delete(listener);
				stores.get(topic)?.listeners.delete(listener);
			};
		},
	};
}
