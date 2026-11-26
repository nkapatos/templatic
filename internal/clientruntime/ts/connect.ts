// Reconnects by hand: an auto-retried EventSource repeats its original URL, but ?u= must carry the current address.

import { apply, type WireEvent } from "./live";
import { markLiveOpen, pageToken, readConfig, recordLive } from "./tc";

const BASE_DELAY_MS = 1000;
const MAX_DELAY_MS = 15000;

function streamURL(reconnect: boolean): string {
	const cfg = readConfig();
	const parts = [`t=${encodeURIComponent(pageToken())}`];
	if (reconnect) parts.push("r=1");
	parts.push(`u=${encodeURIComponent(location.pathname + location.search)}`);
	return `${cfg.stream}?${parts.join("&")}`;
}

/** connect opens the live stream and reconnects with backoff; it throws when the page has no token. */
export function connect(): EventSource {
	if (!pageToken()) {
		throw new Error("tc: live disabled: the page has no token; pass world.PageToken(r) as templatic.BootOptions.Token");
	}
	let torn = false;
	let awaitingFirstFrame = false;
	let delay = BASE_DELAY_MS;

	const open = (reconnect: boolean): EventSource => {
		const es = new EventSource(streamURL(reconnect));
		es.onopen = () => {
			if (torn) awaitingFirstFrame = true;
			else markLiveOpen();
		};
		es.addEventListener("tc", (ev) => {
			// only a frame proves the server accepted the stream, so only then reset the backoff
			delay = BASE_DELAY_MS;
			const wire = JSON.parse((ev as MessageEvent<string>).data) as WireEvent;
			if (awaitingFirstFrame) {
				awaitingFirstFrame = false;
				torn = false;
				// a resync frame is recorded as "resync" by live.ts, not as a resume
				if (wire.kind !== "resync") recordLive("resume");
			}
			apply(wire);
		});
		es.onerror = () => {
			// one outage, one record: the retry loop keeps erroring while the network is down
			es.close();
			awaitingFirstFrame = false;
			if (!torn) {
				torn = true;
				recordLive("disconnect");
			}
			// equal jitter so clients torn together do not reconnect in lockstep
			setTimeout(() => open(true), delay / 2 + Math.random() * (delay / 2));
			delay = Math.min(delay * 2, MAX_DELAY_MS);
		};
		return es;
	};
	return open(false);
}
