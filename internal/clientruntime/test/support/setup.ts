import { afterEach } from "vitest";

// The runtime wires document and window listeners at boot; each test boots a fresh module graph, so the previous one's listeners must go.
const added: Array<() => void> = [];
for (const target of [document, window] as EventTarget[]) {
	const add = target.addEventListener.bind(target);
	target.addEventListener = (type: string, fn: EventListenerOrEventListenerObject, opts?: boolean | AddEventListenerOptions): void => {
		add(type, fn, opts);
		added.push(() => target.removeEventListener(type, fn, opts));
	};
}

afterEach(() => {
	for (const remove of added.splice(0)) remove();
	document.head.innerHTML = "";
	document.body.innerHTML = "";
	history.replaceState(null, "", "/workspace");
});
