import { Popover, Tooltip } from "bootstrap";
import { defineHook } from "templatic";

// Tooltips and popovers have no data API: each needs an instance per
// element. destroyed disposes it, which removes a tooltip that is showing
// when its element leaves the page.
defineHook("bs-tooltip", {
	mounted({ el }) {
		new Tooltip(el);
	},
	// A redraw changed data-bs-title; the instance read it at creation.
	updated({ el }) {
		const title = el.getAttribute("data-bs-title") ?? "";
		Tooltip.getInstance(el)?.setContent({ ".tooltip-inner": title });
	},
	destroyed({ el }) {
		Tooltip.getInstance(el)?.dispose();
	},
});

defineHook("bs-popover", {
	mounted({ el }) {
		new Popover(el);
	},
	destroyed({ el }) {
		Popover.getInstance(el)?.dispose();
	},
});
