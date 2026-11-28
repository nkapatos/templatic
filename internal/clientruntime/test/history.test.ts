import { beforeEach, describe, expect, test, vi } from "vitest";

import { configScript } from "./support/config";
import { stubFetch, type Reply } from "./support/fetch";
import { settle } from "./support/frame";

let commands: typeof import("../ts/commands");
let historyMod: typeof import("../ts/history");
let reload: ReturnType<typeof vi.spyOn>;

beforeEach(async () => {
	vi.useFakeTimers();
	vi.spyOn(console, "warn").mockImplementation(() => {});
	reload = vi.spyOn(window.location, "reload").mockImplementation(() => {});
	vi.resetModules();
	commands = await import("../ts/commands");
	historyMod = await import("../ts/history");
	const tc = await import("../ts/tc");
	document.body.innerHTML = `${configScript({ token: undefined })}<div data-tc-region="daily.surface"><p>day</p></div>`;
	tc.markReady();
});

function pushTwo(): void {
	commands.applyPush("daily.open", { args: { day: "2026-01-05" } }, "/daily/2026-01-05");
	commands.applyPush("daily.open", { args: { day: "2026-01-07" } }, "/daily/2026-01-07");
	expect(location.pathname).toBe("/daily/2026-01-07");
}

function pushed(reply: Reply | Promise<Reply>): ReturnType<typeof stubFetch> {
	const calls = stubFetch(reply);
	pushTwo();
	historyMod.wire();
	return calls;
}

describe("replay pairs", () => {
	test("pairOf accepts a replay pair and carries its intent", () => {
		const p = commands.pairOf({ tc: 1, action: "daily.open", args: { day: "2026-01-05" } });
		expect(p?.action).toBe("daily.open");
		expect((p?.args as { day: string }).day).toBe("2026-01-05");
	});

	test("pairOf refuses null, unversioned and foreign entries", () => {
		expect(commands.pairOf(null)).toBeNull();
		expect(commands.pairOf({})).toBeNull();
		expect(commands.pairOf({ tc: 2, action: "x" })).toBeNull();
		expect(commands.pairOf({ action: "x" })).toBeNull();
	});
});

describe("push", () => {
	test("a server-declared url pushes its entry with the replay pair", () => {
		const before = history.length;
		commands.applyPush("daily.open", { args: { day: "2026-01-05" } }, "/daily/2026-01-05");
		expect(history.length).toBe(before + 1);
		const p = commands.pairOf(history.state);
		expect(p?.action).toBe("daily.open");
		expect(location.href).toBe("http://page.local/daily/2026-01-05");
		expect(commands.lastNavRecord().decision).toBe("push");
	});

	test("a cross-origin declaration is refused and recorded, never thrown", () => {
		const before = history.length;
		commands.applyPush("daily.open", {}, "https://elsewhere.example/daily");
		expect(history.length).toBe(before);
		expect(commands.lastNavRecord().decision).toBe("ignored");
		expect(location.href).toBe("http://page.local/workspace");
	});
});

describe("pop", () => {
	test("a popped pair re-dispatches its intent through the server", async () => {
		const calls = pushed({ status: 200, body: { url: "/daily/2026-01-05" } });
		history.back();
		await settle();
		expect(calls).toHaveLength(1);
		expect(calls[0].json().action).toBe("daily.open");
		expect((calls[0].json().args as { day: string }).day).toBe("2026-01-05");
		expect(commands.lastNavRecord().decision).toBe("replay");
	});

	test("a replay's corrected url replaces the entry the tab is on", async () => {
		pushed({ status: 200, body: { url: "/daily/2026-01-30" } });
		history.back();
		await settle();
		expect(location.href).toBe("http://page.local/daily/2026-01-30");
		expect(commands.lastNavRecord().decision).toBe("replay");
	});

	test("a stale replay response cannot replace a newer pop's entry", async () => {
		const stale = Promise.withResolvers<Reply>();
		const current = Promise.withResolvers<Reply>();
		const calls = stubFetch(stale.promise, current.promise);
		pushTwo();
		historyMod.wire();
		history.back();
		await settle();
		history.forward();
		await settle();
		// replays run one at a time: the newer waits for the stale one to settle
		expect(calls).toHaveLength(1);
		stale.resolve({ status: 200, body: { url: "/daily/2026-99-99" } });
		await settle();
		expect(location.pathname).toBe("/daily/2026-01-07");
		expect(commands.lastNavRecord().decision).toBe("push");
		expect(calls).toHaveLength(2);
		current.resolve({ status: 200, body: { url: "/daily/2026-01-07" } });
		await settle();
		expect(commands.lastNavRecord().decision).toBe("replay");
		history.back();
		expect(location.pathname).toBe("/daily/2026-01-05");
	});

	test("the response of the entry the tab IS on still applies", async () => {
		const only = Promise.withResolvers<Reply>();
		pushed(only.promise);
		history.back();
		await settle();
		only.resolve({ status: 200, body: { url: "/daily/2026-01-05" } });
		await settle();
		expect(commands.lastNavRecord().decision).toBe("replay");
	});

	test("an entry without a pair reloads: the URL routes to truth", async () => {
		const calls = stubFetch({ status: 200, body: { url: "/x" } });
		commands.applyPush("daily.open", { args: { day: "2026-01-05" } }, "/daily/2026-01-05");
		historyMod.wire();
		history.back();
		await settle();
		expect(reload).toHaveBeenCalledTimes(1);
		expect(calls).toHaveLength(0);
		expect(commands.lastNavRecord().decision).toBe("reload");
	});

	test("a replay the server rejects falls back to the same GET", async () => {
		const calls = pushed({ status: 500 });
		history.back();
		await settle();
		expect(calls).toHaveLength(1);
		expect(reload).toHaveBeenCalledTimes(1);
		expect(commands.lastNavRecord().decision).toBe("reload");
	});

	test("a replay that normalizes to the current address does not churn history", async () => {
		pushed({ status: 200, body: { url: "/daily/2026-01-05" } });
		const before = history.length;
		history.back();
		await settle();
		expect(history.length).toBe(before);
		expect(commands.lastNavRecord().decision).toBe("replay");
	});
});

test("recordNav is the single observability channel", () => {
	commands.recordNav("replay", "/x");
	expect(commands.lastNavRecord().decision).toBe("replay");
	commands.recordNav("push", "/y");
	expect(commands.lastNavRecord().url).toBe("/y");
});

describe("replace", () => {
	test("Replace and the current URL replace the entry; a new URL pushes", () => {
		commands.applyPush("search", { args: { q: "a" } }, "/search?q=a");
		const n = history.length;
		commands.applyPush("search", { args: { q: "ke" } }, "/search?q=ke", true);
		expect([history.length, location.search, (history.state as { args: unknown }).args]).toEqual([n, "?q=ke", { q: "ke" }]);
		commands.applyPush("search", { args: { q: "ke" } }, "/search?q=ke");
		expect([history.length, commands.lastNavRecord().decision]).toEqual([n, "replace"]);
		commands.applyPush("search", { args: { q: "keys" } }, "/search?q=keys");
		expect([history.length, commands.lastNavRecord().decision]).toEqual([n + 1, "push"]);
	});
});

describe("navigation order", () => {
	test("links run one at a time and only the newest waiting one runs", async () => {
		document.body.innerHTML += `<a id="a" href="/a" data-tc-action="go" data-tc-args='{"to":"a"}'>a</a>
			<a id="b" href="/b" data-tc-action="go" data-tc-args='{"to":"b"}'>b</a>
			<a id="c" href="/c" data-tc-action="go" data-tc-args='{"to":"c"}'>c</a>`;
		const first = Promise.withResolvers<Reply>();
		const calls = stubFetch(first.promise, { status: 200, body: { url: "/c" } });
		const actions = await import("../ts/actions");
		actions.wire();
		for (const id of ["a", "b", "c"]) document.getElementById(id)!.click();
		await settle();
		expect(calls).toHaveLength(1);
		first.resolve({ status: 200, body: { url: "/a" } });
		await settle();
		expect(calls.map((c) => c.json().args)).toEqual([{ to: "a" }, { to: "c" }]);
		expect(location.pathname).toBe("/c");
	});
});
