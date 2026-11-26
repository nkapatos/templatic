import type { Manifest } from "./tc";
import { pageToken, readConfig, tcPhase, tcState } from "./tc";
import { onSwapped } from "./live";
import { connect } from "./connect";
import { wire as wireActions } from "./actions";
import { wireForms } from "./forms";
import { scanHooks, wireHooks } from "./hooks";
import { loadEntry, registerEntries, requestModules, scanModules } from "./modules";
import { register as registerIslands, scan as scanIslands } from "./islands";

(globalThis as Record<string, unknown>).tcState = tcState;

async function phase(p: Parameters<typeof tcPhase>[0], fn: () => void | Promise<void>): Promise<void> {
	try {
		await fn();
		tcPhase(p);
	} catch (err) {
		tcPhase(p, err);
		console.error(`tc: ${p} phase failed — continuing without it; window.tcState() carries the record`, err);
	}
}

async function boot(): Promise<void> {
	// Config is the one hard phase: without it nothing has endpoints.
	try {
		readConfig();
		tcPhase("config");
	} catch (err) {
		tcPhase("config", err);
		console.error("tc: config phase failed — no endpoints, boot aborts; window.tcState() carries the record", err);
		return;
	}

	const cfg = readConfig();
	const manifestURL = new URL(`${cfg.assets}/tc.json`, document.baseURI);
	await phase("manifest", async () => {
		let manifest: Manifest = cfg.manifest ?? {};
		if (!cfg.manifest) {
			try {
				const res = await fetch(manifestURL);
				if (res.ok) manifest = (await res.json()) as Manifest;
				else console.warn("tc: manifest unavailable; islands stay server-rendered");
			} catch (err) {
				console.warn("tc: manifest fetch failed; islands stay server-rendered", err);
			}
		}
		registerIslands(manifest, manifestURL.href);
		registerEntries(manifest, manifestURL.href);
	});

	// app runs before the island and hook scans so its hooks exist at first paint.
	await phase("app", () => loadEntry("app"));
	await phase("modules", () => {
		void requestModules(cfg.modules ?? []);
		void scanModules(document);
	});

	// onSwapped is registered before connect so the first live frame's island hosts hydrate.
	// Chunk loading is not awaited: a slow or failed chunk must not delay connect.
	await phase("actions", () => wireActions());
	await phase("forms", () => wireForms());
	await phase("islands", () => {
		onSwapped((region) => {
			void scanIslands(region);
			void scanModules(region);
		});
		void scanIslands(document);
	});
	await phase("hooks", () => {
		wireHooks();
		scanHooks(document);
	});
	// A page rendered without page tokens (no World) is static: it has no live channel to open.
	if (pageToken()) await phase("live", () => void connect());
}

void boot();
