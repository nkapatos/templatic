import { swapTargetMissing, swapsAttr } from "./actions";
import { CommandError, postForm, submitKey } from "./commands";
import { isReady } from "./tc";

/** formBody encodes entries as a native submit would: CRLF line breaks, file fields as their name. */
export function formBody(entries: Iterable<[string, FormDataEntryValue]>): string {
	const crlf = (v: string): string => v.replace(/\r\n|\r|\n/g, "\r\n");
	const out = new URLSearchParams();
	for (const [name, value] of entries) {
		out.append(crlf(name), crlf(value instanceof File ? value.name : value));
	}
	return out.toString();
}

interface FailElement {
	id: string;
	textContent: string | null;
	getAttribute(name: string): string | null;
	setAttribute(name: string, value: string): void;
	removeAttribute(name: string): void;
}

/** FailForm is the form surface failure handling touches. */
export interface FailForm {
	setAttribute(name: string, value: string): void;
	removeAttribute(name: string): void;
	hasAttribute(name: string): boolean;
	querySelectorAll(selector: string): ArrayLike<FailElement>;
	requestSubmit(submitter?: HTMLElement | null): void;
}

const FALLBACK = "data-tc-fallback";
const SUBMIT = "_tc_submit";
// records the slot id we added to aria-describedby so clearing removes only it
const DESCRIBED = "data-tc-described";
let slotSeq = 0;

function attrValue(name: string): string {
	return name.replace(/["\\]/g, "\\$&");
}

function each(list: ArrayLike<FailElement>, fn: (el: FailElement) => void): void {
	for (let i = 0; i < list.length; i++) fn(list[i]);
}

/** clearFormErrors clears the marks and slot messages of a previous failed submit. */
export function clearFormErrors(form: FailForm): void {
	form.removeAttribute("data-tc-error");
	each(form.querySelectorAll('[aria-invalid="true"]'), (el) => el.removeAttribute("aria-invalid"));
	each(form.querySelectorAll(`[${DESCRIBED}]`), (el) => {
		const id = el.getAttribute(DESCRIBED) ?? "";
		const rest = (el.getAttribute("aria-describedby") ?? "").split(/\s+/).filter((t) => t && t !== id);
		if (rest.length) el.setAttribute("aria-describedby", rest.join(" "));
		else el.removeAttribute("aria-describedby");
		el.removeAttribute(DESCRIBED);
	});
	each(form.querySelectorAll("[data-tc-field-error], [data-tc-form-error]"), (el) => {
		el.textContent = "";
	});
}

/**
 * formFailure marks a 400 on the form and shows another 4xx's message in its form error slot, falls back to a native
 * submit on 0, 409 or 5xx, and returns what it did.
 */
export function formFailure(form: FailForm, err: unknown, submitter: HTMLElement | null = null): "marked" | "fallback" | "ignored" {
	if (err instanceof CommandError && err.status > 400 && err.status < 500 && err.status !== 409 && err.formError) {
		clearFormErrors(form);
		form.setAttribute("data-tc-error", err.formError);
		each(form.querySelectorAll("[data-tc-form-error]"), (el) => {
			el.textContent = err.formError;
		});
		return "marked";
	}
	if (err instanceof CommandError && err.status === 400) {
		clearFormErrors(form);
		form.setAttribute("data-tc-error", err.message);
		for (const [name, message] of Object.entries(err.fields)) {
			const slot = form.querySelectorAll(`[data-tc-field-error="${attrValue(name)}"]`)[0];
			if (slot) {
				slot.textContent = message;
				if (!slot.id) slot.id = `tc-field-error-${++slotSeq}`;
			}
			each(form.querySelectorAll(`[name="${attrValue(name)}"]`), (el) => {
				el.setAttribute("aria-invalid", "true");
				if (!slot) return;
				const tokens = (el.getAttribute("aria-describedby") ?? "").split(/\s+/).filter(Boolean);
				if (!tokens.includes(slot.id)) tokens.push(slot.id);
				el.setAttribute("aria-describedby", tokens.join(" "));
				el.setAttribute(DESCRIBED, slot.id);
			});
		}
		each(form.querySelectorAll("[data-tc-form-error]"), (el) => {
			el.textContent = err.formError;
		});
		return "marked";
	}
	const status = err instanceof CommandError ? err.status : 0;
	// 409: the session is gone, so the input goes out natively rather than being lost.
	if ((status === 0 || status === 409 || status >= 500) && !form.hasAttribute(FALLBACK)) {
		form.setAttribute(FALLBACK, "");
		form.requestSubmit(submitter);
		return "fallback";
	}
	return "ignored";
}

// forms with a command in flight; a repeat submit is swallowed until it settles
const inflight = new WeakSet<object>();

// The submission id of a form rendered without one (it arrived in a swap): a runtime key until the first answer
// brings a server id. Kept per form, so a retry of the same submission is recognized.
const ids = new WeakMap<HTMLFormElement, string>();

/** wireForms delegates submits of data-tc-form forms to commands. */
export function wireForms(): void {
	document.addEventListener("submit", (ev) => {
		if (ev.defaultPrevented) return;
		const form = ev.target instanceof HTMLFormElement ? ev.target : null;
		if (!form?.dataset.tcForm) return;
		// formFailure's resubmit: the browser posts it natively
		if (form.hasAttribute(FALLBACK)) return;

		// no live session: submit natively rather than queue
		if (!isReady()) return;
		// the command swaps a region this page lacks: the native post renders the page that has it
		if (swapTargetMissing(form, document)) {
			console.debug(`tc: ${form.dataset.tcForm} swaps ${form.getAttribute(swapsAttr)}, absent here — posting natively`);
			return;
		}
		ev.preventDefault();
		if (inflight.has(form)) return;
		inflight.add(form);
		const action = form.dataset.tcForm;
		const data = new FormData(form, ev.submitter);
		// a morph may have replaced the button, and requestSubmit refuses one outside the form
		const submitter = ev.submitter;
		const fallbackSubmitter = (): HTMLElement | null =>
			submitter && submitter.isConnected && (submitter as HTMLButtonElement).form === form ? submitter : null;
		const field = form.querySelector<HTMLInputElement>(`input[name="${SUBMIT}"]`);
		if (!field?.value) {
			let id = ids.get(form);
			if (!id) ids.set(form, (id = submitKey()));
			data.set(SUBMIT, id);
		}
		// the form posts to its own action, as it would natively; the submission id rides in its fields
		const body = form.hasAttribute("data-tc-multipart") ? data : formBody(data.entries());
		const sent = postForm(action, form.getAttribute("action") ?? "", body, {
			onSubmit: (id) => {
				if (field) field.value = id;
				else ids.set(form, id);
			},
		});
		void sent
			.then(
				() => clearFormErrors(form),
				(err) => {
					if (formFailure(form, err, fallbackSubmitter()) === "ignored") {
						console.warn(`tc: form command ${action} failed`, err);
					}
				},
			)
			.finally(() => inflight.delete(form));
	});
}
