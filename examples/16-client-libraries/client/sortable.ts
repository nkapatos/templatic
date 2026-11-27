import Sortable from "sortablejs";
import { defineHook } from "templatic";
import { send } from "templatic/commands";

// SortableJS moves the dragged item in the DOM; the drop sends the move as
// the typed queue.move command. The server's redraw then matches the DOM,
// so the morph moves nothing; it settles any difference (a concurrent move
// elsewhere) to the server's order. A refused move puts the items back.
defineHook("sortable", {
	mounted({ el }) {
		if (!(el instanceof HTMLElement)) return;
		let before: string[] = [];
		const sortable = Sortable.create(el, {
			handle: ".grip",
			dataIdAttr: "data-id",
			onStart: () => {
				before = sortable.toArray();
			},
			onEnd: (e) => {
				const id = e.item.dataset.id;
				if (!id || e.newIndex === undefined || e.newIndex === e.oldIndex) return;
				send("queue.move", { id, to: e.newIndex }).catch(() => sortable.sort(before));
			},
		});
	},
	destroyed({ el }) {
		if (el instanceof HTMLElement) Sortable.get(el)?.destroy();
	},
});
