import { defineHook } from "templatic";
import { send } from "templatic/commands";

// Typing settles for this long before the table follows it.
const debounceMs = 300;

// Without JS the form GETs the page with the search in the URL. The hook
// raises orders.view instead, on submit and once typing settles, so the
// table swaps in place and the URL is pushed. After every other
// navigation (sort, paging, back and forward) the fields follow the URL.
defineHook("search", {
	mounted({ el, signal, on }) {
		if (!(el instanceof HTMLFormElement)) return;
		const sort = el.elements.namedItem("sort");
		const dir = el.elements.namedItem("dir");
		const q = el.elements.namedItem("q");
		if (!(sort instanceof HTMLInputElement && dir instanceof HTMLInputElement && q instanceof HTMLInputElement)) return;
		let timer: ReturnType<typeof setTimeout> | undefined;
		let inflight = 0;
		signal.addEventListener("abort", () => clearTimeout(timer));

		const search = (): void => {
			clearTimeout(timer);
			inflight++;
			send("orders.view", { page: 1, q: q.value, sort: sort.value, dir: dir.value })
				.catch((err) => console.warn("search: orders.view failed", err))
				.finally(() => inflight--);
		};
		el.addEventListener("submit", (ev) => {
			ev.preventDefault();
			search();
		}, { signal });
		q.addEventListener("input", () => {
			clearTimeout(timer);
			timer = setTimeout(() => {
				const here = new URL(location.href).searchParams.get("q") ?? "";
				if (q.value.trim() !== here) search();
			}, debounceMs);
		}, { signal });
		on("nav", ({ url }) => {
			const params = new URL(url, location.href).searchParams;
			sort.value = params.get("sort") ?? "";
			dir.value = params.get("dir") ?? "";
			// A push from this hook's own search: keep what is being typed.
			if (inflight > 0) return;
			const next = params.get("q") ?? "";
			if (q.value.trim() !== next) q.value = next;
		});
	},
});
