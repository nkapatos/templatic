import { defineHook } from "templatic";

// Kind is the hook's args: how to format the element's value.
type Kind = "number" | "currency" | "datetime" | "relative";

const units: [Intl.RelativeTimeFormatUnit, number][] = [["second", 60], ["minute", 60], ["hour", 24], ["day", Infinity]];

// ago formats the time since ms, in the largest unit that keeps it above 1.
function ago(ms: number): string {
	let n = (ms - Date.now()) / 1000;
	for (const [unit, size] of units) {
		if (Math.abs(n) < size) return new Intl.RelativeTimeFormat(undefined, { numeric: "auto" }).format(Math.round(n), unit);
		n /= size;
	}
	return "";
}

// format writes el's value, read from its value or datetime attribute, in
// the browser's locale; a value it cannot read keeps the server's text.
function format(el: Element, kind: Kind): void {
	const raw = el.getAttribute(kind === "datetime" || kind === "relative" ? "datetime" : "value") ?? "";
	const n = kind === "number" || kind === "currency" ? Number(raw) : Date.parse(raw);
	if (raw === "" || Number.isNaN(n)) return;
	switch (kind) {
		case "number":
			el.textContent = new Intl.NumberFormat().format(n);
			break;
		case "currency":
			el.textContent = new Intl.NumberFormat(undefined, { style: "currency", currency: el.getAttribute("data-currency") ?? "EUR" }).format(n);
			break;
		case "datetime":
			el.textContent = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "medium" }).format(n);
			break;
		case "relative":
			el.textContent = ago(n);
			break;
	}
}

// The server renders a plain fallback text; the hook replaces it on mount
// and again after every morph, which puts the server's text back first.
defineHook<Kind>("intl", {
	mounted({ el, args, signal }) {
		format(el, args);
		if (args !== "relative") return;
		const t = setInterval(() => format(el, args), 1000);
		signal.addEventListener("abort", () => clearInterval(t));
	},
	updated({ el, args }) {
		format(el, args);
	},
});
