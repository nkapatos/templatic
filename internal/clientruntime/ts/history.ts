import { pairOf, recordNav, send, sendForm } from "./commands";
import { navigate } from "./nav";

/** wire mounts the popstate handler that replays a pushed entry's command or reloads; idempotent. */
export function wire(): void {
	if (historyWired) return;
	historyWired = true;
	window.addEventListener("popstate", (ev) => {
		const serial = ++popSerial;
		const pair = pairOf(ev.state);
		if (!pair) {
			recordNav("reload", location.href);
			location.reload();
			return;
		}
		// the browser already moved onto this entry, so the replay must not push a URL
		navigate(() =>
			pair.form !== undefined
				? sendForm(pair.action, pair.form, { navHandled: true })
				: send(pair.action, pair.args, { navHandled: true }),
		)
			.then((raw) => {
				if (raw === undefined || popSerial !== serial) return; // a later pop owns this entry
				const url = (raw as { url?: string }).url;
				if (typeof url !== "string" || url === "") {
					recordNav("replay", location.href);
					return;
				}
				const target = new URL(url, location.href);
				if (target.origin !== location.origin) {
					recordNav("replay", location.href); // apply, never move cross-origin
					return;
				}
				if (target.href !== location.href) {
					history.replaceState(pair, "", target.href);
				}
				recordNav("replay", target.href);
			})
			.catch(() => {
				// the receipt no longer redeems: GET the URL instead
				recordNav("reload", location.href);
				location.reload();
			});
	});
}

let historyWired = false;
let popSerial = 0;
