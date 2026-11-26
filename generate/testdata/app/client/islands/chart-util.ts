// Only one export is imported; the other must be tree-shaken out.
export const UNUSED_SYMBOL = "treeshake-me-out";

export function usedHelper(el: unknown): void {
	console.log("helper", el);
}
