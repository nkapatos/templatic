import { send } from "./commands";
import { wire as wireHistory } from "./history";

/** swapsAttr names the attribute declaring a trigger's swap region. */
export const swapsAttr = "data-tc-swaps";

export interface TriggerLike {
	getAttribute(name: string): string | null;
}
export interface DocumentLike {
	querySelector(selector: string): unknown;
}

/** swapTargetMissing reports whether trigger declares a swap region doc does not host. */
export function swapTargetMissing(trigger: TriggerLike, doc: DocumentLike): boolean {
	const topic = trigger.getAttribute(swapsAttr);
	if (!topic) return false;
	return doc.querySelector(`[data-tc-region="${topic}"]`) === null;
}

/** wire delegates clicks on data-tc-action triggers to commands. */
export function wire(): void {
	document.addEventListener("click", (ev) => {
		if (ev.defaultPrevented) return;
		const trigger = ev.target instanceof Element ? ev.target.closest<HTMLElement>("[data-tc-action]") : null;
		if (!trigger) return;

		// Navigate instead of swallowing a click whose swap this page cannot host.
		if (swapTargetMissing(trigger, document)) {
			console.debug(`tc: ${trigger.dataset.tcAction} swaps ${trigger.getAttribute(swapsAttr)}, absent here — following the href`);
			return;
		}
		ev.preventDefault();

		const action = trigger.dataset.tcAction!;
		const raw = trigger.dataset.tcArgs;
		let args: unknown;
		if (raw) {
			try {
				args = JSON.parse(raw);
			} catch {
				console.warn(`tc: invalid args on action ${action}`);
				return;
			}
		}
		void send(action, args).catch((err) => {
			console.warn(`tc: command ${action} failed`, err);
		});
	});

	wireHistory();
}
