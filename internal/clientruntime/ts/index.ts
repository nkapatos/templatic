import { send as rawSend } from "./commands";
import { scanHooks } from "./hooks";
import { scan as scanIslands } from "./islands";
import { tcState, type TcState } from "./tc";

export { cell, on, onReady } from "./tc";
export type { Cell, LiveEvent, LiveRecord, TcEvents, TcState, Unsubscribe } from "./tc";
export { collection } from "./data";
export type { Collection, DataOp } from "./data";
export { CommandError } from "./commands";
export type { IslandContext } from "./context";
export { defineHook } from "./hooks";
export type { HookCallbacks, HookContext } from "./hooks";

/** Send raises a command; it resolves with the handler's result and rejects with a CommandError. */
export function send<R = unknown>(action: string, args?: unknown): Promise<R | null> {
	return rawSend(action, args) as Promise<R | null>;
}

/** State returns the runtime's window.tcState() snapshot. */
export function state(): TcState {
	return tcState();
}

/** Scan mounts islands and hooks under root. */
export function scan(root: ParentNode): Promise<void> {
	scanHooks(root);
	return scanIslands(root);
}
