export interface DevFrame {
	what?: string;
	css?: Record<string, string>;
	message?: string;
}

/** DevLink is the link surface swapCSS touches. */
export interface DevLink {
	getAttribute(name: string): string | null;
	setAttribute(name: string, value: string): void;
	removeAttribute(name: string): void;
}

const OVERLAY = "tc-dev-overlay";

/** swapCSS rewrites every stylesheet href ending in "/"+old to the new file and returns the count. */
export function swapCSS(links: ArrayLike<DevLink>, map: Record<string, string>): number {
	let n = 0;
	for (let i = 0; i < links.length; i++) {
		const link = links[i];
		const href = link.getAttribute("href");
		if (!href) continue;
		const [path, query = ""] = href.split(/(?=[?#])/, 2);
		for (const [old, next] of Object.entries(map)) {
			if (path === old || path.endsWith("/" + old)) {
				link.setAttribute("href", path.slice(0, path.length - old.length) + next + query);
				// a stale SRI hash would block the new file
				link.removeAttribute("integrity");
				n++;
				break;
			}
		}
	}
	return n;
}

/** applyDev handles a dev frame: reload, css swap, or build-error overlay. */
export function applyDev(f: DevFrame): void {
	switch (f.what) {
		case "reload":
			location.reload();
			break;
		case "css":
			document.getElementById(OVERLAY)?.remove();
			swapCSS(document.querySelectorAll('link[rel="stylesheet"]'), f.css ?? {});
			break;
		case "error": {
			let el = document.getElementById(OVERLAY);
			if (!el) {
				el = document.createElement("pre");
				el.id = OVERLAY;
				el.setAttribute("role", "alert");
				el.style.cssText =
					"position:fixed;inset:auto 0 0 0;max-height:50vh;overflow:auto;margin:0;padding:1rem;" +
					"background:#1b1b1b;color:#ff8a80;font:12px/1.5 ui-monospace,monospace;z-index:2147483647;white-space:pre-wrap";
				document.body.appendChild(el);
			}
			el.textContent = "templatic: build failed\n\n" + (f.message ?? "");
			break;
		}
	}
}
