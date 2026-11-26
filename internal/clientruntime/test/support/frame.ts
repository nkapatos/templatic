import { vi } from "vitest";

import type { WireEvent } from "../../ts/live";

/** settle runs the animation frame and the async flush behind it; the test must hold fake timers. */
export async function settle(): Promise<void> {
	await vi.advanceTimersByTimeAsync(20);
}

/** frame applies wire events and settles the frame they schedule. */
export async function frame(...events: WireEvent[]): Promise<void> {
	const { apply } = await import("../../ts/live");
	for (const ev of events) apply(ev);
	await settle();
}
