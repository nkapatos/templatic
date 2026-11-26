import { vi } from "vitest";

/** FakeEventSource is the server end of the live stream, driven by the test. */
export class FakeEventSource {
	static instances: FakeEventSource[] = [];
	onopen: (() => void) | null = null;
	onerror: (() => void) | null = null;
	closed = false;
	private listeners: Array<(ev: { data: string }) => void> = [];

	constructor(readonly url: string) {
		FakeEventSource.instances.push(this);
	}

	/** install makes it the page's EventSource and forgets earlier instances. */
	static install(): void {
		FakeEventSource.instances = [];
		vi.stubGlobal("EventSource", FakeEventSource);
	}

	static get last(): FakeEventSource {
		return FakeEventSource.instances[FakeEventSource.instances.length - 1];
	}

	addEventListener(kind: string, fn: (ev: { data: string }) => void): void {
		if (kind === "tc") this.listeners.push(fn);
	}

	close(): void {
		this.closed = true;
	}

	open(): void {
		this.onopen?.();
	}

	error(): void {
		this.onerror?.();
	}

	frame(obj: Record<string, unknown>): void {
		for (const fn of this.listeners) fn({ data: JSON.stringify(obj) });
	}
}
