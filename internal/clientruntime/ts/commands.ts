import { emit, isReady, onReady, readConfig, sessionID } from "./tc";

/** Payload is what a command carries: JSON args, or a form's urlencoded fields. */
export interface Payload {
	args?: unknown;
	form?: string;
}

interface CommandBody extends Payload {
	sid: string;
	action: string;
	submit?: string;
}

interface ResultEnvelope {
	result?: unknown;
	url?: string;
	replace?: boolean;
}

/** ReplayPair is the receipt stored on a pushed history entry; tc is the pair format version. */
export interface ReplayPair {
	tc: number;
	action: string;
	args?: unknown;
	form?: string;
}

const HISTORY_VERSION = 1;

/** CommandError is what send rejects with; status is the HTTP status (0 for a network error). */
export class CommandError extends Error {
	readonly status: number;
	readonly fields: Readonly<Record<string, string>>;
	readonly formError: string;
	constructor(action: string, status: number, input?: { fields?: Record<string, string>; formError?: string }) {
		const fields = input?.fields ?? {};
		const formError = input?.formError ?? "";
		super(formError || Object.values(fields)[0] || `live: command ${action} failed (${status})`);
		this.name = "CommandError";
		this.status = status;
		this.fields = fields;
		this.formError = formError;
	}
}

async function failure(action: string, res: Response): Promise<CommandError> {
	if (res.status !== 400) {
		// a 4xx says why in plain text (live.Status, live.NotFound); 5xx and 409 carry nothing to show
		if (res.status > 400 && res.status < 500 && res.status !== 409) {
			const text = await res.text().catch(() => "");
			if (text.trim()) return new CommandError(action, res.status, { formError: text.trim() });
		}
		return new CommandError(action, res.status);
	}
	try {
		const env = (await res.json()) as { error?: { message?: string; fields?: Record<string, string> } };
		return new CommandError(action, 400, { fields: env.error?.fields, formError: env.error?.message });
	} catch {
		return new CommandError(action, 400); // a plain-text 400 (bad payload): nothing to mark
	}
}

/** pairOf reads the replay pair off a history entry, or null when the entry is not ours. */
export function pairOf(state: unknown): ReplayPair | null {
	if (typeof state !== "object" || state === null) return null;
	const candidate = state as { tc?: unknown; action?: unknown; form?: unknown };
	if (candidate.tc !== HISTORY_VERSION || typeof candidate.action !== "string") return null;
	if (candidate.form !== undefined && typeof candidate.form !== "string") return null;
	return state as ReplayPair;
}

/** NavRecord is the URL layer's last navigation decision. */
export interface NavRecord {
	decision: "push" | "replace" | "replay" | "reload" | "ignored";
	url: string;
}
let lastNav: NavRecord = { decision: "ignored", url: "" };

/** recordNav records a navigation decision and emits nav for push, replace and replay. */
export function recordNav(decision: NavRecord["decision"], url: string): void {
	lastNav = { decision, url };
	if (decision === "push" || decision === "replace" || decision === "replay") emit("nav", { url });
}

/** lastNavRecord returns the last navigation decision. */
export function lastNavRecord(): NavRecord {
	return { ...lastNav };
}

/** SendOptions tunes a dispatch for the runtime's internal replays. */
export interface SendOptions {
	/** navHandled makes send resolve with the whole envelope; the caller applies the url. */
	navHandled?: boolean;
	/** noReload rejects on 409 (session gone) instead of reloading. */
	noReload?: boolean;
	/** submit is the form's submission id, sent beside the payload so history replays never carry it. */
	submit?: string;
	/** onSubmit receives the fresh submission id a successful form command answers with. */
	onSubmit?: (id: string) => void;
}

// Sends queued until the session is ready; each entry rejects its promise and leaves the ready queue.
const pending = new Set<() => void>();

function whenReady(fire: () => void, abort: () => void): void {
	if (isReady()) {
		fire();
		return;
	}
	const cancel = onReady(() => {
		pending.delete(entry);
		fire();
	});
	const entry = (): void => {
		cancel();
		abort();
	};
	pending.add(entry);
}

/** abandonPending rejects every queued send and upload. */
export function abandonPending(): void {
	for (const abort of [...pending]) {
		pending.delete(abort);
		abort();
	}
}

/** send raises a command once the session is ready and resolves with the handler's result (null if none). */
export function send(action: string, args?: unknown, opts?: SendOptions): Promise<unknown> {
	return raise(action, args === undefined ? {} : { args }, opts);
}

/** sendForm is send with form, the urlencoded body the browser would POST natively. */
export function sendForm(action: string, form: string, opts?: SendOptions): Promise<unknown> {
	return raise(action, { form }, opts);
}

function raise(action: string, payload: Payload, opts?: SendOptions): Promise<unknown> {
	return new Promise((resolve, reject) => {
		const fire = (): void => {
			const cfg = readConfig();
			const body: CommandBody = { sid: sessionID(), action, ...payload };
			if (opts?.submit) body.submit = opts.submit;
			emit("command", { action, phase: "sent" });
			void fetch(cfg.commands, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify(body),
			}).then(async (res) => {
				if (res.status === 409) {
					// unknown session: reload re-renders and reconnects; the promise still rejects
					emit("command", { action, phase: "error", status: 409 });
					if (!opts?.noReload) location.reload();
					reject(new CommandError(action, 409));
					return;
				}
				if (!res.ok) {
					const err = await failure(action, res);
					emit("command", { action, phase: "error", status: res.status });
					reject(err);
					return;
				}
				emit("command", { action, phase: "ok", status: res.status });
				const next = res.headers?.get("X-TC-Submit");
				if (next) opts?.onSubmit?.(next);
				if (res.status === 204) {
					resolve(opts?.navHandled ? { result: null } : null);
					return;
				}
				let envelope: ResultEnvelope;
				try {
					envelope = (await res.json()) as ResultEnvelope;
				} catch {
					resolve(opts?.navHandled ? { result: null } : null);
					return;
				}
				if (typeof envelope.url === "string" && !opts?.navHandled) {
					applyPush(action, payload, envelope.url, envelope.replace === true);
				}
				resolve(opts?.navHandled ? envelope : (envelope.result ?? null));
			}, () => {
				emit("command", { action, phase: "error", status: 0 });
				reject(new CommandError(action, 0));
			});
		};

		whenReady(fire, () => reject(new CommandError(action, 0)));
	});
}

/** submitKey returns a fresh run-once key: a send or form submission carrying it runs at most once per session. */
export function submitKey(): string {
	// getRandomValues, unlike randomUUID, exists outside secure contexts (plain HTTP to a LAN host)
	const bytes = crypto.getRandomValues(new Uint8Array(16));
	return "c." + Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

/** FormPostOptions carries a form's submission id handling. */
export interface FormPostOptions {
	/** onSubmit receives the fresh submission id the answer carries. */
	onSubmit?: (id: string) => void;
}

/**
 * postForm POSTs a form's body to its action URL with the session in X-TC-Sid and X-TC-Form marking the runtime:
 * the server answers 204 or the error. A redirect rejects with status 0, so the form falls back to a native post.
 */
export function postForm(action: string, url: string, body: FormData | string, opts?: FormPostOptions): Promise<void> {
	return new Promise((resolve, reject) => {
		const fire = (): void => {
			emit("command", { action, phase: "sent" });
			const headers: Record<string, string> = { "X-TC-Sid": sessionID(), "X-TC-Form": "1" };
			if (typeof body === "string") headers["Content-Type"] = "application/x-www-form-urlencoded";
			void fetch(url, { method: "POST", headers, body, redirect: "manual" }).then(async (res) => {
				if (res.type === "opaqueredirect" || (res.status >= 300 && res.status < 400)) {
					emit("command", { action, phase: "error", status: 0 });
					reject(new CommandError(action, 0));
					return;
				}
				const next = res.headers?.get("X-TC-Submit");
				if (next) opts?.onSubmit?.(next);
				if (!res.ok) {
					const err = await failure(action, res);
					emit("command", { action, phase: "error", status: res.status });
					reject(err);
					return;
				}
				emit("command", { action, phase: "ok", status: res.status });
				resolve();
			}, () => {
				emit("command", { action, phase: "error", status: 0 });
				reject(new CommandError(action, 0));
			});
		};
		whenReady(fire, () => reject(new CommandError(action, 0)));
	});
}

/**
 * applyPush moves to a server-declared same-origin URL with its receipt: a new history entry, or the current one
 * replaced when replace is set or the URL is the current one. Other origins are refused and recorded.
 */
export function applyPush(action: string, payload: Payload, url: string, replace = false): void {
	try {
		const target = new URL(url, location.href);
		if (target.origin !== location.origin) {
			recordNav("ignored", url);
			console.warn(`tc: refusing cross-origin push ${url}`);
			return;
		}
		const pair: ReplayPair = { tc: HISTORY_VERSION, action, ...payload };
		// the same address again is no new place to go back to
		if (replace || target.href === location.href) {
			history.replaceState(pair, "", target.href);
			recordNav("replace", target.href);
			return;
		}
		history.pushState(pair, "", target.href);
		recordNav("push", target.href);
	} catch (err) {
		recordNav("ignored", url);
		console.warn(`tc: pushState failed for ${url}`, err);
	}
}
