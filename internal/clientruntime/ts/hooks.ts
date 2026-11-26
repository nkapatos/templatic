import { scope, type IslandContext, type Mounted } from "./context";
import { on, recordListenerError } from "./tc";

/** HookContext is the scoped context a hook callback receives, plus its element and parsed args. */
export interface HookContext<A = unknown> extends IslandContext {
	el: Element;
	args: A;
}

/** HookCallbacks is the optional lifecycle a hook implements; a throw is recorded and contained. */
export interface HookCallbacks<A = unknown> {
	mounted?(ctx: HookContext<A>): void;
	/** beforeUpdate runs before a morph changes the element; ctx.args are still the old ones. */
	beforeUpdate?(ctx: HookContext<A>): void;
	updated?(ctx: HookContext<A>): void;
	destroyed?(ctx: HookContext<A>): void;
	disconnected?(ctx: HookContext<A>): void;
	reconnected?(ctx: HookContext<A>): void;
}

interface Instance {
	name: string;
	ctx: HookContext;
	scope: Mounted;
	def: HookCallbacks;
	updating: boolean;
}

const defs = new Map<string, HookCallbacks>();
const instances = new Map<Element, Instance>();
const SELECTOR = "[data-tc-hook]";

function readArgs(el: Element): unknown {
	const raw = el.getAttribute("data-tc-hook-args");
	if (raw === null || raw === "") return null;
	try {
		return JSON.parse(raw);
	} catch (err) {
		recordListenerError(`hook args (${el.getAttribute("data-tc-hook")})`, err);
		return null;
	}
}

function call(inst: Instance, phase: keyof HookCallbacks): void {
	const fn = inst.def[phase];
	if (!fn) return;
	try {
		fn.call(inst.def, inst.ctx);
	} catch (err) {
		recordListenerError(`hook ${inst.name}.${phase}`, err);
	}
}

function insideIsland(el: Element): boolean {
	for (let n = el.parentNode as (ParentNode & Partial<Element>) | null; n; n = n.parentNode as typeof n) {
		if (typeof n.getAttribute === "function" && n.getAttribute("data-tc-island") !== null) return true;
	}
	return false;
}

function mount(el: Element): void {
	const name = el.getAttribute("data-tc-hook") ?? "";
	const def = defs.get(name);
	if (!def) return; // pending: defineHook resolves it later
	const sc = scope();
	const ctx = Object.assign(Object.create(null) as object, sc.ctx, { el, args: readArgs(el) }) as HookContext;
	const inst: Instance = { name, ctx, scope: sc, def, updating: false };
	instances.set(el, inst);
	call(inst, "mounted");
}

function destroy(el: Element): void {
	const inst = instances.get(el);
	if (!inst) return;
	instances.delete(el);
	inst.scope.dispose();
	call(inst, "destroyed");
}

function hookElements(root: ParentNode): Element[] {
	const out: Element[] = [];
	const self = root as Partial<Element>;
	if (typeof self.getAttribute === "function" && self.getAttribute("data-tc-hook") !== null) out.push(root as Element);
	for (const el of root.querySelectorAll(SELECTOR)) out.push(el);
	return out;
}

/** scanHooks mounts every defined, unmounted hook under root; idempotent. */
export function scanHooks(root: ParentNode = document): void {
	for (const el of hookElements(root)) {
		if (insideIsland(el)) continue;
		const inst = instances.get(el);
		if (inst && inst.name === el.getAttribute("data-tc-hook")) continue;
		if (inst) destroy(el);
		mount(el);
	}
}

/** defineHook defines a hook and mounts matching elements already on the page. */
export function defineHook<A = unknown>(name: string, callbacks: HookCallbacks<A>): void {
	if (defs.has(name)) {
		console.warn(`tc: hook ${name} defined twice — keeping the first`);
		return;
	}
	defs.set(name, callbacks as HookCallbacks);
	if (typeof document !== "undefined") scanHooks(document);
}

/** morphCallbacks are the idiomorph callbacks that drive hook updates and removal. */
export const morphCallbacks = {
	beforeNodeMorphed(oldNode: Node, newNode: Node): boolean {
		const inst = instances.get(oldNode as Element);
		if (inst && (oldNode as Element).outerHTML !== (newNode as Element).outerHTML) {
			inst.updating = true;
			call(inst, "beforeUpdate");
		}
		return true;
	},
	afterNodeMorphed(oldNode: Node): void {
		const el = oldNode as Element;
		const inst = instances.get(el);
		if (!inst) return;
		if (inst.name !== el.getAttribute("data-tc-hook")) {
			destroy(el);
			mount(el);
			return;
		}
		if (!inst.updating) return;
		inst.updating = false;
		inst.ctx.args = readArgs(el);
		call(inst, "updated");
	},
	beforeNodeRemoved(node: Node): boolean {
		if (typeof (node as Partial<Element>).querySelectorAll === "function" || typeof (node as Partial<Element>).getAttribute === "function") {
			destroyUnder(node as unknown as ParentNode);
		}
		return true;
	},
};

/** destroyUnder destroys every mounted hook at or under root. */
export function destroyUnder(root: ParentNode): void {
	const self = root as Partial<Element>;
	if (typeof self.getAttribute === "function" && instances.has(root as Element)) destroy(root as Element);
	if (typeof root.querySelectorAll !== "function") return;
	for (const el of root.querySelectorAll(SELECTOR)) destroy(el);
}

let wired = false;

/** wireHooks routes live-channel disconnect and resume to hooks. */
export function wireHooks(): void {
	if (wired) return;
	wired = true;
	on("disconnect", () => {
		for (const inst of [...instances.values()]) call(inst, "disconnected");
	});
	on("resume", () => {
		for (const inst of [...instances.values()]) call(inst, "reconnected");
	});
}
