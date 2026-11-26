import { Idiomorph } from "idiomorph";
import { vi } from "vitest";

export interface MorphRecord {
	topic: string;
	style: string | undefined;
	/** islandsPresent counts island hosts under the target when the morph starts. */
	islandsPresent: number;
}

/** recordMorphs spies on the real Idiomorph.morph, which still runs; failNext makes the next morph throw instead. */
export function recordMorphs(): { morphs: MorphRecord[]; failNext(message: string): void } {
	const real = Idiomorph.morph.bind(Idiomorph);
	const morphs: MorphRecord[] = [];
	let failure: string | null = null;
	vi.spyOn(Idiomorph, "morph").mockImplementation(((target: Element, html: string, opts?: { morphStyle?: string }) => {
		if (failure !== null) {
			const message = failure;
			failure = null;
			throw new Error(message);
		}
		morphs.push({
			topic: target.getAttribute("data-tc-region") ?? "",
			style: opts?.morphStyle,
			islandsPresent: target.querySelectorAll("[data-tc-island]").length,
		});
		return real(target, html, opts as never);
	}) as never);
	return { morphs, failNext: (message) => (failure = message) };
}
