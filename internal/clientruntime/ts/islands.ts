import { bindsOfTag, latestCell, registerBinds, tcIsland, type Manifest } from "./tc";

const chunks = new Map<string, string>();
const styles = new Map<string, string[]>();
const loading = new Map<string, Promise<void>>();
const styleLinks = new Set<string>();

const LOAD_TIMEOUT_MS = 10000;

/** register records the chunk and css URLs of the manifest's islands/<tag> modules, resolved against baseURL. */
export function register(manifest: Manifest, baseURL: string): void {
	for (const [name, mod] of Object.entries(manifest.modules ?? {})) {
		if (!name.startsWith("islands/")) continue;
		const tag = name.slice("islands/".length);
		if (mod.file) chunks.set(tag, new URL(mod.file, baseURL).href);
		const css = (mod.css ?? []).map((u) => new URL(u, baseURL).href);
		if (css.length > 0) styles.set(tag, css);
	}
	registerBinds(manifest);
}

/** scan loads the chunks of islands under root in parallel; it settles when all finish and never rejects. */
export async function scan(root: ParentNode = document): Promise<void> {
	const tags = new Set<string>();
	for (const el of root.querySelectorAll<HTMLElement>("[data-tc-island]")) {
		tags.add((el.dataset.tcIsland || el.tagName).toLowerCase());
	}
	await Promise.allSettled([...tags].map(load));
}

// the timeout keeps a chunk that never defines its element from hanging its scan
function importAndDefine(tag: string, url: string): Promise<unknown> {
	let timer: ReturnType<typeof setTimeout> | undefined;
	const timeout = new Promise<never>((_, reject) => {
		timer = setTimeout(() => reject(new Error(`island ${tag} not defined within ${LOAD_TIMEOUT_MS}ms`)), LOAD_TIMEOUT_MS);
	});
	const loaded = import(url).then(() => customElements.whenDefined(tag));
	return Promise.race([loaded, timeout]).finally(() => clearTimeout(timer));
}

function loadStyles(tag: string): void {
	if (styleLinks.has(tag)) return;
	styleLinks.add(tag);
	for (const href of styles.get(tag) ?? []) {
		const link = document.createElement("link");
		link.rel = "stylesheet";
		link.href = href;
		document.head.appendChild(link);
	}
}

async function load(tag: string): Promise<void> {
	if (customElements.get(tag)) return;
	const url = chunks.get(tag);
	if (!url) {
		console.warn(`tc: island ${tag} missing from the manifest`);
		return;
	}
	loadStyles(tag);
	let pending = loading.get(tag);
	if (!pending) {
		// a failed load leaves the cache so a later scan retries
		const attempt: Promise<void> = importAndDefine(tag, url)
			.then(() => {
				tcIsland(tag);
				reseedBinds(tag);
			})
			.catch((err) => {
				if (loading.get(tag) === attempt) loading.delete(tag);
				tcIsland(tag, err);
				console.warn(`tc: island ${tag} failed to load`, err);
			});
		pending = attempt;
		loading.set(tag, pending);
	}
	await pending;
}

// a cell event that raced the chunk load arrived before the tag was defined; reapply the latched value
function reseedBinds(tag: string): void {
	const binds = bindsOfTag(tag);
	if (binds.length === 0) return;
	for (const host of document.querySelectorAll(`[data-tc-island="${tag}"]`)) {
		const bound = host as unknown as { tcBind?: (field: string, v: unknown) => void };
		for (const b of binds) {
			const last = latestCell(b.topic);
			if (last.known) bound.tcBind?.(b.field, last.value);
		}
	}
}
