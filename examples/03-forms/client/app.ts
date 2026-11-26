// The app entry: loaded on every page.
import { defineHook } from "templatic";

import "./app.css";

// After a successful command the runtime leaves the form's fields as they
// are; the new-book form is cleared so the next entry starts blank.
defineHook<string>("reset-on-ok", {
	mounted({ el, args, on }) {
		on("command", (d) => {
			if (d.action === args && d.phase === "ok") el.querySelector("form")?.reset();
		});
	},
});
