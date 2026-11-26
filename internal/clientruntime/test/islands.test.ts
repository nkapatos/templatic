import { beforeEach, expect, test, vi } from "vitest";

let islands: typeof import("../ts/islands");
let tc: typeof import("../ts/tc");

beforeEach(async () => {
	// only the load timeout is faked: the chunk imports run on the real event loop
	vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
	vi.spyOn(console, "warn").mockImplementation(() => {});
	vi.resetModules();
	islands = await import("../ts/islands");
	tc = await import("../ts/tc");
});

const chunk = (src: string) => `data:text/javascript,${encodeURIComponent(src)}`;

test("a chunk that never defines its tag fails on timeout without blocking the others, and is retried", async () => {
	islands.register(
		{
			version: 2,
			modules: {
				"islands/x-good": { file: chunk('customElements.define("x-good", class extends HTMLElement {})') },
				"islands/x-hang": { file: chunk("export {}") },
				"modules/not-an-island": { file: chunk("export {}") },
			},
		},
		"http://localhost/dist/tc.json",
	);
	document.body.innerHTML = `<x-hang data-tc-island="x-hang"></x-hang><x-good data-tc-island="x-good"></x-good>`;
	const whenDefined = vi.spyOn(customElements, "whenDefined");
	const hangLoads = () => whenDefined.mock.calls.filter(([tag]) => tag === "x-hang").length;

	const scanned = islands.scan(document);
	await vi.waitFor(() => expect(customElements.get("x-good")).toBeDefined());
	await vi.advanceTimersByTimeAsync(10000);
	await scanned;
	expect(tc.tcState().phases["island:x-hang"]).toBe("failed");
	expect(tc.tcState().lastErrors["island:x-hang"]).toContain("x-hang");
	expect(tc.tcState().phases).not.toHaveProperty("island:x-good");

	const before = hangLoads();
	const retry = islands.scan(document);
	await vi.waitFor(() => expect(hangLoads()).toBe(before + 1));
	customElements.define("x-hang", class extends HTMLElement {});
	await retry;
	expect(tc.tcState().phases).not.toHaveProperty("island:x-hang");
});
