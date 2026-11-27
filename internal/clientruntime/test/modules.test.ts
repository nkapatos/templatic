import { beforeEach, expect, test, vi } from "vitest";

import { frame } from "./support/frame";

let modules: typeof import("../ts/modules");
let live: typeof import("../ts/live");
let tc: typeof import("../ts/tc");

let seq = 0;
// every url is unique: the runtime caches a module by url, so a repeat would not run again
const mod = (tag: string) => `data:text/javascript,(globalThis.__tcRan ??= []).push(${JSON.stringify(tag)});//${seq++}`;
const ran = () => (globalThis as unknown as { __tcRan: string[] }).__tcRan;

beforeEach(async () => {
	vi.useFakeTimers();
	vi.stubGlobal("__tcRan", []);
	vi.spyOn(console, "error").mockImplementation(() => {});
	vi.resetModules();
	modules = await import("../ts/modules");
	live = await import("../ts/live");
	tc = await import("../ts/tc");
});

function setup(files: Record<string, string>): void {
	const entries: Record<string, { file?: string }> = {};
	for (const [name, file] of Object.entries(files)) entries[name] = { file };
	modules.registerEntries({ version: 2, modules: entries }, "http://x/_templatic/tc.json");
}

test("modules: app loads once and runs before the promise resolves", async () => {
	setup({ app: mod("app") });
	await modules.loadEntry("app");
	await modules.loadEntry("app");
	expect(ran()).toEqual(["app"]);
});

test("modules: an entry's CSS is linked once, also when Head linked it", async () => {
	document.head.innerHTML = `<link rel="stylesheet" href="/_templatic/app-1.css">`;
	modules.registerEntries(
		{ version: 2, modules: { app: { file: mod("app"), css: ["app-1.css", "app-2.css"] } } },
		"http://page.local/_templatic/tc.json",
	);
	await modules.loadEntry("app");
	const hrefs = Array.from(document.head.querySelectorAll<HTMLLinkElement>('link[rel="stylesheet"]'), (l) => l.href);
	expect(hrefs).toEqual(["http://page.local/_templatic/app-1.css", "http://page.local/_templatic/app-2.css"]);
});

test("modules: only app and modules/<name> entries register, from the modules map", async () => {
	setup({ "islands/x-a": mod("island"), "modules/m": mod("m") });
	await modules.requestModules(["m"]);
	await modules.loadEntry("islands/x-a");
	expect(ran()).toEqual(["m"]);
});

test("modules: an absent app.ts is a no-op", async () => {
	setup({});
	await modules.loadEntry("app");
	expect(ran()).toEqual([]);
});

test("modules: requested modules load in parallel, once each", async () => {
	setup({ "modules/a": mod("a"), "modules/b": mod("b") });
	await modules.requestModules(["a", "b", "a"]);
	expect([...ran()].sort()).toEqual(["a", "b"]);
});

test("modules: a marker in a swapped-in region loads its module once", async () => {
	setup({ "modules/charts": mod("charts") });
	document.body.innerHTML = `<div data-tc-region="r"></div>`;
	const pending: Array<Promise<void>> = [];
	live.onSwapped((region) => pending.push(modules.scanModules(region)));
	await frame({ kind: "swap", topic: "r", html: `<template data-tc-module="charts"></template><template data-tc-module="charts"></template>` });
	await frame({ kind: "swap", topic: "r", html: `<p></p><template data-tc-module="charts"></template>` });
	await Promise.all(pending);
	expect(ran()).toEqual(["charts"]);
});

test("modules: a failing module is contained and recorded", async () => {
	setup({ "modules/bad": "data:text/javascript,throw new Error('mod boom')", "modules/ok": mod("ok") });
	await modules.requestModules(["bad", "ok"]);
	expect(ran()).toEqual(["ok"]);
	expect(tc.tcState().phases.modules).toBe("failed");
	expect(tc.tcState().lastErrors.modules ?? "").toContain("mod boom");
});
