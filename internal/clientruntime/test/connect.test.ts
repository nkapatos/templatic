import { beforeEach, describe, expect, test, vi } from "vitest";

import { configScript, TOKEN } from "./support/config";
import { FakeEventSource } from "./support/event-source";
import { settle } from "./support/frame";
import { recordMorphs } from "./support/morph";

let connect: typeof import("../ts/connect");
let tc: typeof import("../ts/tc");

const PAGE = `
<div data-tc-region="panel"><p>old</p></div>
<div data-tc-collection="drops.list" data-tc-ver="1"><div data-tc-item="a">A</div></div>
<span data-tc-cell="note.open">n1</span>
`;

beforeEach(async () => {
	vi.useFakeTimers();
	// full jitter: a reconnect waits exactly the backoff
	vi.spyOn(Math, "random").mockReturnValue(1);
	vi.spyOn(console, "error").mockImplementation(() => {});
	FakeEventSource.install();
	vi.resetModules();
	connect = await import("../ts/connect");
	tc = await import("../ts/tc");
	document.body.innerHTML = configScript() + PAGE;
});

function connected(): FakeEventSource {
	return connect.connect() as unknown as FakeEventSource;
}

// tear drops the stream and waits out the backoff; it returns the reconnect
async function tear(es: FakeEventSource, backoff = 1000): Promise<FakeEventSource> {
	const n = FakeEventSource.instances.length;
	es.error();
	await vi.advanceTimersByTimeAsync(backoff);
	expect(es.closed).toBe(true);
	expect(FakeEventSource.instances).toHaveLength(n + 1);
	return FakeEventSource.last;
}

describe("resume", () => {
	test("a torn stream records disconnect, a normal reattach frame records resume and converges the page", async () => {
		const { morphs } = recordMorphs();
		const first = connected();
		const before = tc.tcState().live;
		first.open();
		first.frame({ kind: "cell", topic: "note.open", v: "n1" });
		first.frame({ kind: "ready" });
		await settle();
		expect(tc.tcState().live.connection).toBe("open");

		let es = await tear(first);
		expect(tc.tcState().live.connection).toBe("torn");
		es = await tear(es, 2000);
		expect(tc.tcState().live.disconnects).toBe(before.disconnects + 1);

		es.open();
		expect(tc.tcState().live.resumes).toBe(before.resumes);
		es.frame({ kind: "cell", topic: "note.open", v: "n3" });
		es.frame({ kind: "swap", topic: "panel", html: "<p>three</p>" });
		es.frame({ kind: "ready" });
		await settle();
		const st = tc.tcState().live;
		expect(st.connection).toBe("open");
		expect(st.resumes).toBe(before.resumes + 1);
		expect(st.resyncs).toBe(before.resyncs);
		expect(document.querySelector('[data-tc-cell="note.open"]')?.textContent).toBe("n3");
		expect(document.querySelector('[data-tc-region="panel"]')?.textContent).toBe("three");
		expect(morphs).toHaveLength(1);
		expect(st.events.slice(-2).map((e) => e.type)).toEqual(["disconnect", "resume"]);
	});

	test("ops missed while torn arrive after the reconnect and apply from the version the page holds", async () => {
		const first = connected();
		first.open();
		first.frame({ kind: "ready" });
		const es = await tear(first);
		es.open();
		es.frame({ kind: "ready" });
		es.frame({ kind: "ops", topic: "drops.list", ver: 3, ops: [{ op: "u", k: "a", html: `<div data-tc-item="a">A</div>` }, { op: "u", k: "b", html: `<div data-tc-item="b">B</div>` }] });
		await settle();
		expect([...document.querySelectorAll("[data-tc-item]")].map((e) => e.getAttribute("data-tc-item"))).toEqual(["b", "a"]);
	});

	test("grace expiry: the resync frame reloads and is recorded, never claimed as a resume", async () => {
		const reload = vi.spyOn(window.location, "reload").mockImplementation(() => {});
		const first = connected();
		first.open();
		first.frame({ kind: "ready" });
		const before = tc.tcState().live;
		const es = await tear(first);
		es.open();
		es.frame({ kind: "resync" });
		expect(reload).toHaveBeenCalledTimes(1);
		const st = tc.tcState().live;
		expect(st.resyncs).toBe(before.resyncs + 1);
		expect(st.resumes).toBe(before.resumes);
	});

	test("live transitions feed the internal event feed the public bus will subscribe to", async () => {
		const seen: string[] = [];
		tc.onLiveEvent((ev) => seen.push(ev.type));
		const first = connected();
		first.open();
		first.frame({ kind: "ready" });
		const es = await tear(first);
		es.open();
		es.frame({ kind: "ready" });
		expect(seen).toEqual(["disconnect", "resume"]);
	});
});

describe("stream url", () => {
	test("every connect reports the page's current address; reconnects say so and follow live.At moves", async () => {
		const first = connected();
		first.open();
		first.frame({ kind: "ready" });
		expect(first.url).toContain(`u=${encodeURIComponent("/workspace")}`);
		expect(first.url).not.toContain("r=1");
		history.pushState({ tc: 1, action: "x" }, "", "/notes/n2?filter=all");
		const es = await tear(first);
		expect(es.url).toContain("r=1");
		expect(es.url).toContain(`u=${encodeURIComponent("/notes/n2?filter=all")}`);
	});

	test("with a page token the stream connects with ?t= and the sid comes from the token", () => {
		const token = `${btoa(JSON.stringify({ s: "srv-minted_sid", i: 1 })).replace(/=+$/, "")}.c2ln`;
		document.body.innerHTML = configScript({ token });
		connected();
		const url = FakeEventSource.instances[0].url;
		expect(url.startsWith(`/live/stream?t=${encodeURIComponent(token)}&`)).toBe(true);
		expect(url).not.toContain("sid=");
		expect(tc.sessionID()).toBe("srv-minted_sid");
	});

	test("without a token connect throws, opens no stream, and names BootOptions.Token", () => {
		document.body.innerHTML = configScript({ token: undefined });
		expect(() => connect.connect()).toThrow("BootOptions.Token");
		expect(() => connect.connect()).toThrow("world.PageToken(r)");
		expect(FakeEventSource.instances).toHaveLength(0);
		expect(tc.sessionID()).toBe("");
	});

	test("a malformed token yields no sid (the server rejects it; the client does not guess)", () => {
		expect(tc.sidFromToken("not-a-token")).toBe("");
		expect(tc.sidFromToken("")).toBe("");
		expect(tc.sidFromToken(TOKEN)).toBe("test-sid");
	});
});

describe("back/forward cache", () => {
	function pageEvent(type: "pagehide" | "pageshow", persisted: boolean): void {
		const ev = new Event(type);
		Object.defineProperty(ev, "persisted", { value: persisted });
		window.dispatchEvent(ev);
	}

	test("pagehide closes the stream and schedules no reconnect, so a cached page holds no connection", async () => {
		const es = connected();
		es.open();
		es.frame({ kind: "ready" });
		pageEvent("pagehide", true);
		expect(es.closed).toBe(true);
		expect(tc.tcState().live.connection).toBe("torn");
		await vi.advanceTimersByTimeAsync(60_000);
		expect(FakeEventSource.instances).toHaveLength(1);
	});

	test("pagehide cancels a pending reconnect", async () => {
		const es = connected();
		es.open();
		es.error();
		pageEvent("pagehide", true);
		await vi.advanceTimersByTimeAsync(60_000);
		expect(FakeEventSource.instances).toHaveLength(1);
	});

	test("a restore from the cache reconnects at once as a reconnect and records the resume", async () => {
		const first = connected();
		first.open();
		first.frame({ kind: "ready" });
		const before = tc.tcState().live;
		pageEvent("pagehide", true);
		pageEvent("pageshow", true);
		expect(FakeEventSource.instances).toHaveLength(2);
		const es = FakeEventSource.last;
		expect(es.url).toContain("r=1");
		es.open();
		es.frame({ kind: "ready" });
		const st = tc.tcState().live;
		expect(st.connection).toBe("open");
		expect(st.disconnects).toBe(before.disconnects + 1);
		expect(st.resumes).toBe(before.resumes + 1);
	});

	test("a restore after the session expired gets the server's resync, which reloads", () => {
		const reload = vi.spyOn(window.location, "reload").mockImplementation(() => {});
		connected().open();
		pageEvent("pagehide", true);
		pageEvent("pageshow", true);
		FakeEventSource.last.open();
		FakeEventSource.last.frame({ kind: "resync" });
		expect(reload).toHaveBeenCalledTimes(1);
	});

	test("a first pageshow and a pagehide that unloads open nothing more", async () => {
		const es = connected();
		pageEvent("pageshow", false);
		expect(FakeEventSource.instances).toHaveLength(1);
		pageEvent("pagehide", false);
		expect(es.closed).toBe(true);
		await vi.advanceTimersByTimeAsync(60_000);
		expect(FakeEventSource.instances).toHaveLength(1);
	});

	test("a restore while the stream is open opens no second one", () => {
		connected().open();
		pageEvent("pageshow", true);
		expect(FakeEventSource.instances).toHaveLength(1);
	});
});

describe("backoff", () => {
	// reconnectsAfter asserts the reconnect lands exactly ms after the drop
	async function reconnectsAfter(es: FakeEventSource, ms: number): Promise<FakeEventSource> {
		const n = FakeEventSource.instances.length;
		es.error();
		await vi.advanceTimersByTimeAsync(ms - 1);
		expect(FakeEventSource.instances).toHaveLength(n);
		await vi.advanceTimersByTimeAsync(1);
		expect(FakeEventSource.instances).toHaveLength(n + 1);
		return FakeEventSource.last;
	}

	test("reconnect backoff doubles when the server opens then drops, resets on a frame, and is jittered", async () => {
		let es = connected();
		for (const ms of [1000, 2000, 4000]) {
			es.open();
			es = await reconnectsAfter(es, ms);
		}

		es.open();
		es.frame({ kind: "ready" });
		es = await reconnectsAfter(es, 1000);

		vi.mocked(Math.random).mockReturnValue(0);
		await reconnectsAfter(es, 1000);
	});
});
