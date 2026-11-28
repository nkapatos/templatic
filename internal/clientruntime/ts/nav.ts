// Navigations run one at a time, the newest waiting one winning. Run together, their swaps reach the page in the
// order the server handled them but their URLs in the order the answers came back, so the address and the region
// could end apart. One at a time, both follow the order the navigations were made in.

let running = false;
let waiting: { run: () => void; drop: () => void } | null = null;

/**
 * navigate runs nav once the navigation before it settled. A navigation still waiting when a newer one comes never
 * runs and resolves with undefined.
 */
export function navigate<T>(nav: () => Promise<T>): Promise<T | undefined> {
	return new Promise((resolve, reject) => {
		const run = (): void => {
			running = true;
			nav()
				.then(resolve, reject)
				.finally(() => {
					running = false;
					const next = waiting;
					waiting = null;
					next?.run();
				});
		};
		if (!running) {
			run();
			return;
		}
		waiting?.drop();
		waiting = { run, drop: () => resolve(undefined) };
	});
}
