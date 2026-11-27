import { isReady, onReady, readConfig, sessionID } from "./tc";

// Topics this connection is subscribed to; a new connection starts from what the page holds.
const subscribed = new Set<string>();

// Each collection form reports the topics the page holds, at their versions (-1: no known version).
const sources: Array<() => Map<string, number>> = [];

/** holdsFrom adds a source of the collection topics the page holds and their versions. */
export function holdsFrom(fn: () => Map<string, number>): void {
	sources.push(fn);
}

function held(): Map<string, number> {
	const out = new Map<string, number>();
	for (const fn of sources) {
		for (const [topic, v] of fn()) {
			const prev = out.get(topic);
			out.set(topic, prev === undefined ? v : Math.min(prev, v));
		}
	}
	return out;
}

/** heldParam returns the c= value a new stream carries (topic:ver pairs) and marks those topics subscribed. */
export function heldParam(): string {
	subscribed.clear();
	const pairs: string[] = [];
	for (const [topic, v] of held()) {
		subscribed.add(topic);
		pairs.push(v < 0 ? topic : `${topic}:${v}`);
	}
	return pairs.join(",");
}

let recheckOnReady = false;

/**
 * subscribeMissing subscribes to the topics in topics this connection is not subscribed to yet. Before the session
 * is ready it waits for ready, then subscribes whatever the stream's URL did not carry.
 */
export function subscribeMissing(topics: Map<string, number>): void {
	if (!isReady()) {
		if (!recheckOnReady) {
			recheckOnReady = true;
			onReady(() => {
				recheckOnReady = false;
				subscribeMissing(held());
			});
		}
		return;
	}
	const fresh: Record<string, number> = {};
	let any = false;
	for (const [topic, v] of topics) {
		if (subscribed.has(topic)) continue;
		fresh[topic] = v;
		any = true;
	}
	if (any) post(fresh);
}

/** catchUp asks for what a page at version ver of topic missed; the answer arrives on the stream. */
export function catchUp(topic: string, ver: number): void {
	post({ [topic]: ver });
}

// Before ready there is no session to subscribe; the next connect reports what the page holds anyway.
function post(topics: Record<string, number>): void {
	if (!isReady()) return;
	for (const topic of Object.keys(topics)) subscribed.add(topic);
	const cfg = readConfig();
	void fetch(cfg.commands, {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify({ sid: sessionID(), subscribe: topics }),
	})
		.then((res) => {
			// 409: the server no longer holds the session; the stream's reconnect resyncs the page
			if (!res.ok && res.status !== 409) throw new Error(`subscribe answered ${res.status}`);
		})
		.catch((err: unknown) => {
			for (const topic of Object.keys(topics)) subscribed.delete(topic);
			console.error("tc: collection subscribe failed — the page reloads to catch up", err);
			location.reload();
		});
}
