import { vi } from "vitest";

export interface Reply {
	status: number;
	type?: string;
	body?: unknown;
	headers?: Record<string, string>;
}

export interface Call {
	url: string;
	init: RequestInit & { headers?: Record<string, string> };
	/** json is the request body parsed as JSON. */
	json(): { action?: string; args?: unknown; form?: string; submit?: string };
}

/** stubFetch replaces fetch: call n answers replies[n], the last repeating; a Promise reply is a response the test releases. */
export function stubFetch(...replies: Array<Reply | Promise<Reply>>): Call[] {
	const calls: Call[] = [];
	vi.stubGlobal("fetch", (url: string, init: Call["init"]) => {
		calls.push({ url, init, json: () => JSON.parse(init.body as string) });
		const reply = replies[Math.min(calls.length - 1, replies.length - 1)] ?? { status: 204 };
		return Promise.resolve(reply).then((r) => ({
			status: r.status,
			type: r.type ?? "basic",
			ok: r.status >= 200 && r.status < 300,
			headers: new Headers(r.headers),
			json: () => Promise.resolve(r.body),
			text: () => Promise.resolve(typeof r.body === "string" ? r.body : r.body === undefined ? "" : JSON.stringify(r.body)),
		}));
	});
	return calls;
}
