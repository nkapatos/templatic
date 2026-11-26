export function readProps<T>(el: Element): T | null {
	const script = el.querySelector(
		":scope > script[type='application/tc-props']",
	);
	if (!script?.textContent?.trim()) return null;
	try {
		return JSON.parse(script.textContent) as T;
	} catch (err) {
		console.warn("tc: bad props payload", err);
		return null;
	}
}

export function clearPropsScript(el: Element): void {
	const script = el.querySelector(
		":scope > script[type='application/tc-props']",
	);
	script?.remove();
}
