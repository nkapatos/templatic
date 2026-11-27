import { beforeEach, expect, test, vi } from "vitest";

import { frame } from "./support/frame";

let hooks: typeof import("../ts/hooks");
let tc: typeof import("../ts/tc");

beforeEach(async () => {
	vi.useFakeTimers();
	vi.spyOn(console, "error").mockImplementation(() => {});
	vi.resetModules();
	hooks = await import("../ts/hooks");
	tc = await import("../ts/tc");
	await import("../ts/live");
});

type Args = { t?: string };

function recorder(log: string[]): import("../ts/hooks").HookCallbacks<Args> {
	const rec = (p: string) => (ctx: { args: Args | null; signal: AbortSignal }) =>
		log.push(`${p}:${ctx.args?.t ?? ""}${p === "destroyed" ? (ctx.signal.aborted ? "+aborted" : "+live") : ""}`);
	return { mounted: rec("mounted"), beforeUpdate: rec("beforeUpdate"), updated: rec("updated"), destroyed: rec("destroyed"), disconnected: rec("disconnected"), reconnected: rec("reconnected") };
}

const tip = (t: string, body = "x") => `<span data-tc-hook="tip" data-tc-hook-args='{"t":"${t}"}'>${body}</span>`;

test("hooks: the initial scan mounts with parsed args", () => {
	document.body.innerHTML = `<div>${tip("a")}</div>`;
	const log: string[] = [];
	hooks.defineHook("tip", recorder(log));
	hooks.scanHooks(document);
	expect(log).toEqual(["mounted:a"]);
});

test("hooks: late definition resolves elements already on the page", () => {
	document.body.innerHTML = `<div>${tip("a")}${tip("b")}</div>`;
	hooks.scanHooks(document);
	const log: string[] = [];
	hooks.defineHook("tip", recorder(log));
	expect(log).toEqual(["mounted:a", "mounted:b"]);
});

test("hooks: a kept, changed element gets beforeUpdate + updated with fresh args", async () => {
	document.body.innerHTML = `<div data-tc-region="r">${tip("a")}</div>`;
	const log: string[] = [];
	hooks.defineHook("tip", recorder(log));
	await frame({ kind: "swap", topic: "r", html: tip("b") });
	expect(log).toEqual(["mounted:a", "beforeUpdate:a", "updated:b"]);
});

test("hooks: a kept, unchanged element gets no callbacks", async () => {
	document.body.innerHTML = `<div data-tc-region="r">${tip("a")}<p>1</p></div>`;
	const log: string[] = [];
	hooks.defineHook("tip", recorder(log));
	await frame({ kind: "swap", topic: "r", html: `${tip("a")}<p>2</p>` });
	expect(log).toEqual(["mounted:a"]);
});

test("hooks: removal destroys after aborting the signal", async () => {
	document.body.innerHTML = `<div data-tc-region="r"><div>${tip("a")}</div></div>`;
	const log: string[] = [];
	hooks.defineHook("tip", recorder(log));
	await frame({ kind: "swap", topic: "r", html: "<p>gone</p>" });
	expect(log).toEqual(["mounted:a", "destroyed:a+aborted"]);
});

test("hooks: a changed hook name destroys then mounts", async () => {
	document.body.innerHTML = `<div data-tc-region="r">${tip("a")}</div>`;
	const log: string[] = [];
	hooks.defineHook("tip", recorder(log));
	hooks.defineHook("other", { mounted: () => log.push("other:mounted") });
	await frame({ kind: "swap", topic: "r", html: `<span data-tc-hook="other">x</span>` });
	expect(log.filter((l) => !l.startsWith("beforeUpdate"))).toEqual(["mounted:a", "destroyed:a+aborted", "other:mounted"]);
});

test("hooks: swapped-in and inserted elements mount", async () => {
	document.body.innerHTML = `<div data-tc-region="r"></div><div data-tc-collection="s" data-tc-ver="0"></div>`;
	const log: string[] = [];
	hooks.defineHook("tip", recorder(log));
	await frame({ kind: "swap", topic: "r", html: tip("a") });
	await frame({ kind: "ops", topic: "s", ver: 1, ops: [{ op: "u", k: "k1", html: `<div data-tc-item="k1">${tip("b")}</div>` }] });
	expect(log).toEqual(["mounted:a", "mounted:b"]);
});

test("hooks: elements inside an island host belong to the island", () => {
	document.body.innerHTML = `<x-isl data-tc-island="x-isl">${tip("a")}</x-isl>`;
	const log: string[] = [];
	hooks.defineHook("tip", recorder(log));
	expect(log).toEqual([]);
});

test("hooks: a throwing hook is contained and recorded", () => {
	document.body.innerHTML = `<div>${tip("a")}${tip("b")}</div>`;
	const log: string[] = [];
	hooks.defineHook("tip", {
		mounted(ctx) {
			if ((ctx.args as { t: string }).t === "a") throw new Error("hook boom");
			log.push("mounted:b");
		},
	});
	expect(log).toEqual(["mounted:b"]);
	expect(tc.tcState().lastErrors.listener ?? "").toContain("hook boom");
});

test("hooks: disconnect and resume reach mounted hooks", () => {
	document.body.innerHTML = `<div>${tip("a")}</div>`;
	hooks.wireHooks();
	const log: string[] = [];
	hooks.defineHook("tip", recorder(log));
	tc.recordLive("disconnect");
	tc.recordLive("resume");
	expect(log).toEqual(["mounted:a", "disconnected:a", "reconnected:a"]);
});
