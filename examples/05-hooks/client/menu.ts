import { defineHook } from "templatic";

// The menu is a <details>: it opens, closes and reports its expanded state
// without JS. The hook adds Escape and click-outside.
defineHook("menu", {
	mounted({ el, signal }) {
		if (!(el instanceof HTMLDetailsElement)) return;
		const summary = el.querySelector("summary");
		const close = (refocus: boolean) => {
			if (!el.open) return;
			el.open = false;
			if (refocus) summary?.focus();
		};
		el.addEventListener("keydown", (e) => {
			if (e.key === "Escape") close(true);
		}, { signal });
		// signal aborts when the hook is destroyed, which removes this
		// document listener with it.
		document.addEventListener("click", (e) => {
			if (!el.contains(e.target as Node)) close(false);
		}, { signal });
	},
});
