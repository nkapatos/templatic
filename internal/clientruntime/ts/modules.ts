import { tcPhase, type Manifest } from "./tc";

const entries = new Map<string, string>();
const entryCSS = new Map<string, string[]>();
const loaded = new Map<string, Promise<void>>();

/** registerEntries indexes the manifest's modules (app, modules/<name>) against baseURL. */
export function registerEntries(m: Manifest, baseURL: string): void {
	for (const [name, mod] of Object.entries(m.modules ?? {})) {
		if (name !== "app" && !name.startsWith("modules/")) continue;
		if (mod.file) entries.set(name, new URL(mod.file, baseURL).href);
		entryCSS.set(name, (mod.css ?? []).map((u) => new URL(u, baseURL).href));
	}
}

// linkCSS links an entry's CSS that the page does not link yet: Head already
// links what the rendered page uses.
function linkCSS(name: string): void {
	const linked = new Set(
		Array.from(document.querySelectorAll<HTMLLinkElement>('link[rel="stylesheet"]'), (l) => l.href),
	);
	for (const href of entryCSS.get(name) ?? []) {
		if (linked.has(href)) continue;
		const link = document.createElement("link");
		link.rel = "stylesheet";
		link.href = href;
		document.head.appendChild(link);
	}
}

/** loadEntry loads an entry once; it resolves after it ran and rejects when it failed. */
export function loadEntry(name: string): Promise<void> {
	let p = loaded.get(name);
	if (p) return p;
	const url = entries.get(name);
	// a CSS-only entry has no file, only its stylesheets
	linkCSS(name);
	if (!url) {
		p = Promise.resolve();
		if (name !== "app" && !entryCSS.has(name)) console.warn(`tc: module ${name} missing from the manifest`);
	} else {
		p = import(url).then(() => undefined);
	}
	loaded.set(name, p);
	return p;
}

/** requestModules loads the named page modules in parallel. */
export function requestModules(names: readonly string[]): Promise<void> {
	return Promise.all(
		names.map((n) =>
			loadEntry(`modules/${n}`).catch((err: unknown) => {
				console.error(`tc: module ${n} failed to load — the page continues without it`, err);
				tcPhase("modules", err);
			}),
		),
	).then(() => undefined);
}

/** scanModules loads every module requested by a data-tc-module marker under root. */
export function scanModules(root: ParentNode = document): Promise<void> {
	const names: string[] = [];
	for (const el of root.querySelectorAll("[data-tc-module]")) {
		const n = el.getAttribute("data-tc-module");
		if (n && !names.includes(n)) names.push(n);
	}
	return requestModules(names);
}
