import { Modal } from "bootstrap";
import { defineHook } from "templatic";

// The modal's args name its form's command. The data API opens it; the
// hook preselects the trigger's data-service and closes it after the
// command succeeds.
defineHook<string>("report-modal", {
	mounted({ el, args, on, signal }) {
		const form = el.querySelector("form");
		el.addEventListener("show.bs.modal", (e) => {
			const service = ((e as Modal.Event).relatedTarget as HTMLElement | undefined)?.dataset.service;
			const select = form?.elements.namedItem("service");
			if (service && select instanceof HTMLSelectElement) select.value = service;
		}, { signal });
		on("command", (d) => {
			if (d.action !== args || d.phase !== "ok") return;
			Modal.getInstance(el)?.hide();
			form?.reset();
		});
	},
	destroyed({ el }) {
		Modal.getInstance(el)?.dispose();
	},
});
