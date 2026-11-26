import { defineHook } from "templatic";

// Hand-written twin of components.DialogArgs.
interface DialogArgs {
	id: string;
}

// The hooked link goes to a page with the same content without JS; with JS
// it opens the <dialog> named in args as a modal. showModal gives Escape and
// the focus trap; the hook adds closing on a backdrop click.
defineHook<DialogArgs>("dialog", {
	mounted({ el, args, signal }) {
		const dialog = document.getElementById(args.id);
		if (!(dialog instanceof HTMLDialogElement)) return;
		el.addEventListener("click", (e) => {
			const ev = e as MouseEvent;
			// Let a modified click open the page in a new tab.
			if (ev.button !== 0 || ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) return;
			e.preventDefault();
			dialog.showModal();
		}, { signal });
		// The dialog has no padding, so a click whose target is the dialog
		// itself landed on the backdrop.
		dialog.addEventListener("click", (e) => {
			if (e.target === dialog) dialog.close();
		}, { signal });
	},
});
