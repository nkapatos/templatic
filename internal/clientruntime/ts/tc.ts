/** Config is the page's tc-config payload. */
export interface Config {
	stream: string;
	commands: string;
	assets: string;
	/** manifest, when present, replaces the tc.json fetch. */
	manifest?: Manifest;
	/** token is the server-minted session identity; without it the live channel does not connect. */
	token?: string;
	modules?: string[];
}

/** Manifest is the bundle manifest (tc.json). */
export interface Manifest {
	version?: number;
	mode?: string;
	base?: string;
	modules?: Record<string, ManifestModule>;
	assets?: Record<string, string>;
	integrity?: Record<string, string>;
	binds?: Record<string, ManifestBind[]>;
}

/** ManifestModule is one entry's output, css and static chunk imports; paths are dist-relative. */
export interface ManifestModule {
	file?: string;
	css?: string[];
	imports?: string[];
}

/** ManifestBind pushes a cell topic's value into an island props field. */
export interface ManifestBind {
	topic: string;
	field: string;
}

let config: Config | null = null;
let sid = "";
let ready = false;
const readyListeners: Array<() => void> = [];

/** Phase names a boot phase. */
export type Phase = "config" | "live" | "manifest" | "app" | "modules" | "islands" | "hooks" | "actions" | "forms";
type PhaseStatus = "ok" | "failed";
const phases = new Map<Phase, PhaseStatus>();
const phaseErrors = new Map<Phase, string>();
// island load failures; a successful retry clears the entry
const islandErrors = new Map<string, string>();

/** tcIsland records (err given) or clears an island's load failure in tcState. */
export function tcIsland(tag: string, err?: unknown): void {
	if (err === undefined) islandErrors.delete(tag);
	else islandErrors.set(tag, err instanceof Error ? err.message : String(err));
}

/** tcPhase records a boot phase's outcome; err marks it failed. */
export function tcPhase(p: Phase, err?: unknown): void {
	if (err === undefined) {
		phases.set(p, "ok");
	} else {
		phases.set(p, "failed");
		phaseErrors.set(p, err instanceof Error ? err.message : String(err));
	}
}

/** TcState is the runtime's self-report: phase outcomes, session id, ready state, live record and last errors. */
export interface TcState {
	phases: Record<string, "ok" | "failed">;
	lastErrors: Record<string, string>;
	sid: string;
	ready: boolean;
	live: LiveRecord;
}

/** tcState returns the current TcState. */
export function tcState(): TcState {
	const out = {
		phases: {} as Record<string, PhaseStatus>,
		lastErrors: {} as Record<string, string>,
		sid: sessionID(),
		ready,
		live: liveRecord(),
	};
	for (const [p] of phases) out.phases[p] = phases.get(p)!;
	for (const [p] of phaseErrors) out.lastErrors[p] = phaseErrors.get(p)!;
	for (const [tag, msg] of islandErrors) {
		out.phases[`island:${tag}`] = "failed";
		out.lastErrors[`island:${tag}`] = msg;
	}
	if (listenerError) out.lastErrors.listener = listenerError;
	return out;
}

/** LiveEvent is one recorded live-channel transition. */
export interface LiveEvent {
	type: "disconnect" | "resume" | "resync";
	/** at is epoch ms. */
	at: number;
}
/** LiveRecord counts live-channel transitions and keeps the most recent. */
export interface LiveRecord {
	connection: "connecting" | "open" | "torn";
	disconnects: number;
	resumes: number;
	resyncs: number;
	/** events holds the most recent transitions, oldest first. */
	events: LiveEvent[];
}

const LIVE_EVENT_CAP = 50;
const live: LiveRecord = { connection: "connecting", disconnects: 0, resumes: 0, resyncs: 0, events: [] };
const liveListeners: Array<(ev: LiveEvent) => void> = [];

/** recordLive records a live-channel transition and emits it on the bus. */
export function recordLive(type: LiveEvent["type"]): void {
	const ev: LiveEvent = { type, at: Date.now() };
	switch (type) {
		case "disconnect":
			live.disconnects++;
			live.connection = "torn";
			break;
		case "resume":
			live.resumes++;
			live.connection = "open";
			break;
		case "resync":
			live.resyncs++;
			break;
		default:
			return type satisfies never;
	}
	live.events.push(ev);
	if (live.events.length > LIVE_EVENT_CAP) live.events.shift();
	for (const fn of [...liveListeners]) {
		try {
			fn(ev);
		} catch (err) {
			console.error("tc: live event listener failed", err);
		}
	}
	emit(type, undefined);
}

/** markLiveOpen marks the channel open. */
export function markLiveOpen(): void {
	live.connection = "open";
}

/** onLiveEvent subscribes to live-channel transitions. */
export function onLiveEvent(fn: (ev: LiveEvent) => void): void {
	liveListeners.push(fn);
}

function liveRecord(): LiveRecord {
	return { ...live, events: live.events.map((e) => ({ ...e })) };
}

/** readConfig parses and memoizes the tc-config script; it throws when missing or incomplete. */
export function readConfig(): Config {
	if (config) return config;
	const el = document.querySelector("script[type='application/tc-config']");
	if (!el?.textContent) throw new Error("tc: missing tc-config script");
	const parsed = JSON.parse(el.textContent) as Config;
	if (!parsed.stream || !parsed.commands || !parsed.assets) {
		throw new Error("tc: incomplete tc-config (stream, commands, assets required)");
	}
	config = parsed;
	return config;
}

/** pageToken returns the page token, or "" when none. */
export function pageToken(): string {
	try {
		return readConfig().token ?? "";
	} catch {
		return "";
	}
}

/** sessionID returns the session id read from the page token, or "" when none. */
export function sessionID(): string {
	if (!sid) sid = sidFromToken(pageToken());
	return sid;
}

/** sidFromToken reads the sid from a token's payload without verifying it; the server verifies. */
export function sidFromToken(token: string): string {
	try {
		const body = token.split(".")[0] ?? "";
		const b64 = body.replace(/-/g, "+").replace(/_/g, "/");
		const claims = JSON.parse(atob(b64 + "=".repeat((4 - (b64.length % 4)) % 4))) as { s?: unknown };
		return typeof claims.s === "string" ? claims.s : "";
	} catch {
		return "";
	}
}

/** isReady reports whether the server session is established. */
export function isReady(): boolean {
	return ready;
}

/** markReady marks the session ready, runs queued listeners and emits ready. */
export function markReady(): void {
	if (ready) return;
	ready = true;
	// one throwing listener must not skip the rest or the ready event
	const pending = readyListeners.splice(0);
	for (const fn of pending) {
		try {
			fn();
		} catch (err) {
			recordListenerError("ready listener", err);
		}
	}
	emit("ready", undefined);
}

/** onReady runs fn once the session is ready and returns a cancel for a pending registration. */
export function onReady(fn: () => void): () => void {
	if (ready) {
		fn();
		return () => {};
	}
	readyListeners.push(fn);
	return () => {
		const i = readyListeners.indexOf(fn);
		if (i >= 0) readyListeners.splice(i, 1);
	};
}

/** TcEvents maps each lifecycle event on() subscribes to onto its detail. */
export interface TcEvents {
	/** ready fires once per page load, not again on resume. */
	ready: undefined;
	disconnect: undefined;
	resume: undefined;
	resync: undefined;
	"swap:before": { topic: string; el: Element };
	"swap:after": { topic: string; el: Element };
	append: { topic: string; key: string; el: Element };
	/** item fires per collection item changed; a reset fires once with the container as el and an empty key. */
	item: { topic: string; key: string; el: Element; change: "insert" | "update" | "remove" | "reset" };
	cell: { topic: string; value: unknown };
	"island:mount": { tag: string; host: Element };
	"island:unmount": { tag: string; host: Element };
	/** command status is the HTTP status, 0 for a network failure or a refused redirect. */
	command: { action: string; phase: "sent" | "ok" | "error"; status?: number };
	nav: { url: string };
}

/** Unsubscribe removes a subscription. */
export type Unsubscribe = () => void;

const bus = new Map<keyof TcEvents, Array<(d: never) => void>>();
let listenerError = "";

/** on subscribes to a lifecycle event; listener throws are recorded and never block the wire. */
export function on<E extends keyof TcEvents>(event: E, fn: (detail: TcEvents[E]) => void): Unsubscribe {
	let list = bus.get(event);
	if (!list) {
		list = [];
		bus.set(event, list);
	}
	list.push(fn as (d: never) => void);
	return () => {
		const l = bus.get(event);
		const i = l ? l.indexOf(fn as (d: never) => void) : -1;
		if (l && i >= 0) l.splice(i, 1);
	};
}

/** emit delivers a lifecycle event to its listeners. */
export function emit<E extends keyof TcEvents>(event: E, detail: TcEvents[E]): void {
	const list = bus.get(event);
	if (!list) return;
	for (const fn of [...list]) {
		try {
			(fn as (d: TcEvents[E]) => void)(detail);
		} catch (err) {
			listenerError = `${event}: ${err instanceof Error ? err.message : String(err)}`;
			console.error(`tc: ${event} listener failed — the wire continues`, err);
		}
	}
}

/** recordListenerError records a contained user-code failure in tcState. */
export function recordListenerError(where: string, err: unknown): void {
	listenerError = `${where}: ${err instanceof Error ? err.message : String(err)}`;
	console.error(`tc: ${where} failed — the wire continues`, err);
}

/** Cell is a cell topic's client view. */
export interface Cell<T> {
	get(): T | undefined;
	/** subscribe calls fn with the latest value, if any, then with every new one. */
	subscribe(fn: (v: T) => void): Unsubscribe;
}

/** cell returns the client view of a cell topic. */
export function cell<T = unknown>(topic: string): Cell<T> {
	return {
		get: () => lastCells.get(topic) as T | undefined,
		subscribe(fn) {
			if (lastCells.has(topic)) {
				try {
					fn(lastCells.get(topic) as T);
				} catch (err) {
					listenerError = `cell: ${err instanceof Error ? err.message : String(err)}`;
					console.error("tc: cell subscriber failed", err);
				}
			}
			return on("cell", (d) => {
				if (d.topic === topic) fn(d.value as T);
			});
		},
	};
}

type TopicBind = { tag: string; field: string };
const bindsByTopic = new Map<string, TopicBind[]>();
const bindsByTag = new Map<string, Array<{ topic: string; field: string }>>();
const lastCells = new Map<string, unknown>();

/** registerBinds indexes a manifest's binds by topic and by tag. */
export function registerBinds(m: Manifest): void {
	for (const [tag, list] of Object.entries(m.binds ?? {})) {
		for (const b of list) {
			let topics = bindsByTopic.get(b.topic);
			if (!topics) {
				topics = [];
				bindsByTopic.set(b.topic, topics);
			}
			topics.push({ tag, field: b.field });
			let byTag = bindsByTag.get(tag);
			if (!byTag) {
				byTag = [];
				bindsByTag.set(tag, byTag);
			}
			byTag.push({ topic: b.topic, field: b.field });
		}
	}
}

/** bindsFor returns the bindings listening on a cell topic. */
export function bindsFor(topic: string): TopicBind[] {
	return bindsByTopic.get(topic) ?? [];
}

/** bindsOfTag returns a tag's bindings. */
export function bindsOfTag(tag: string): Array<{ topic: string; field: string }> {
	return bindsByTag.get(tag) ?? [];
}

/** setLatestCell latches a cell topic's last applied value. */
export function setLatestCell(topic: string, v: unknown): void {
	lastCells.set(topic, v);
}

/** latestCell returns the latched value, if the topic has been applied. */
export function latestCell(topic: string): { known: boolean; value: unknown } {
	if (!lastCells.has(topic)) return { known: false, value: undefined };
	return { known: true, value: lastCells.get(topic) };
}
