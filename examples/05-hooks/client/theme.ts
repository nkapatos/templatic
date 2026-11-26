import { defineHook } from "templatic";

// Hand-written twin of components.ThemeArgs.
interface ThemeArgs {
	cookie: string;
	maxAge: number;
}

// Without JS the form POSTs /theme. The hook writes the same cookie and
// repaints in place, so the next page load is painted right by the server.
defineHook<ThemeArgs>("theme", {
	mounted({ el, args, signal }) {
		if (!(el instanceof HTMLFormElement)) return;
		const input = el.elements.namedItem("theme");
		const button = el.querySelector("button");
		if (!(input instanceof HTMLInputElement) || !button) return;
		el.addEventListener("submit", (e) => {
			e.preventDefault();
			const theme = input.value;
			document.documentElement.dataset.theme = theme;
			document.cookie = `${args.cookie}=${theme}; path=/; max-age=${args.maxAge}; samesite=lax`;
			input.value = theme === "dark" ? "light" : "dark";
			button.textContent = `Switch to ${input.value}`;
		}, { signal });
	},
});
