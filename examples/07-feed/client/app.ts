// The app entry: loaded on every page.
import { defineHook } from "templatic";

import "./app.css";

// After a sent message the runtime leaves the fields as they are; the
// message field is cleared and refocused, the name stays.
defineHook<string>("clear-on-ok", {
	mounted({ el, args, on }) {
		on("command", (d) => {
			if (d.action !== args || d.phase !== "ok") return;
			const text = el.querySelector<HTMLInputElement>('input[name="text"]');
			if (!text) return;
			text.value = "";
			text.focus();
		});
	},
});
