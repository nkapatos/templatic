import { send } from "./commands";
import { cell, emit, on, type Cell, type TcEvents, type Unsubscribe } from "./tc";

export interface IslandContext {
	/** Aborts when the island unmounts. */
	signal: AbortSignal;
	/** cell() whose subscriptions end at unmount. */
	cell<T = unknown>(topic: string): Cell<T>;
	/** on() whose listeners end at unmount. */
	on<E extends keyof TcEvents>(event: E, fn: (detail: TcEvents[E]) => void): Unsubscribe;
	send<R = unknown>(action: string, args?: unknown): Promise<R | null>;
}

export interface Mounted {
	ctx: IslandContext;
	dispose(): void;
}

/** Scope returns a context and the dispose that ends its subscriptions and aborts its signal. */
export function scope(onDispose?: () => void): Mounted {
	const ac = new AbortController();
	const subs: Unsubscribe[] = [];
	const track = (u: Unsubscribe): Unsubscribe => {
		subs.push(u);
		return u;
	};
	const ctx: IslandContext = {
		signal: ac.signal,
		cell<T>(topic: string): Cell<T> {
			const c = cell<T>(topic);
			return { get: c.get, subscribe: (fn) => track(c.subscribe(fn)) };
		},
		on: (event, fn) => track(on(event, fn)),
		send: <R>(action: string, args?: unknown) => send(action, args) as Promise<R | null>,
	};
	let disposed = false;
	return {
		ctx,
		dispose() {
			if (disposed) return;
			disposed = true;
			for (const u of subs.splice(0)) u();
			ac.abort();
			onDispose?.();
		},
	};
}

/** MountIsland opens a context for one mount of tag on host; dispose emits island:unmount. */
export function mountIsland(tag: string, host: Element): Mounted {
	return scope(() => emit("island:unmount", { tag, host }));
}

/** Mounted emits island:mount. */
export function mounted(tag: string, host: Element): void {
	emit("island:mount", { tag, host });
}
