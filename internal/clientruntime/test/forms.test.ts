import { beforeEach, describe, expect, test, vi } from "vitest";

import { configScript } from "./support/config";
import { stubFetch } from "./support/fetch";
import { settle } from "./support/frame";

let commands: typeof import("../ts/commands");
let forms: typeof import("../ts/forms");
let live: typeof import("../ts/live");
let tc: typeof import("../ts/tc");

beforeEach(async () => {
	vi.useFakeTimers();
	vi.resetModules();
	commands = await import("../ts/commands");
	forms = await import("../ts/forms");
	live = await import("../ts/live");
	tc = await import("../ts/tc");
	document.body.innerHTML = configScript();
});

function form(inner: string, attrs = ""): HTMLFormElement {
	const el = document.createElement("form");
	el.innerHTML = inner;
	if (attrs) for (const a of attrs.split(" ")) el.setAttribute(a, "");
	document.body.append(el);
	return el;
}

function control(f: HTMLFormElement, selector: string): HTMLElement {
	const el = f.querySelector<HTMLElement>(selector);
	if (!el) throw new Error(`fixture missing ${selector}`);
	return el;
}

function submitSpy(f: HTMLFormElement): ReturnType<typeof vi.spyOn> {
	return vi.spyOn(f, "requestSubmit").mockImplementation(() => {});
}

describe("form body", () => {
	test("form body is the urlencoded body a native submit sends", () => {
		expect(forms.formBody([["day", "2026-11-22"]])).toBe("day=2026-11-22");
		expect(forms.formBody([["tag", "a"], ["n", "1"], ["tag", "b"]])).toBe("tag=a&n=1&tag=b");
		expect(forms.formBody([["q", "a b&c=d"]])).toBe("q=a+b%26c%3Dd");
		expect(forms.formBody([["note", "one\ntwo\rthree\r\nfour"]])).toBe("note=one%0D%0Atwo%0D%0Athree%0D%0Afour");
		expect(forms.formBody([["__proto__", "p"], ["constructor", "c"]])).toBe("__proto__=p&constructor=c");
	});

	test("form body carries a file field as its name (uploads ride the multipart path)", () => {
		expect(forms.formBody([["f", new File(["content"], "a.csv")], ["f", new File([""], "b.csv")]])).toBe("f=a.csv&f=b.csv");
	});
});

describe("failure handling", () => {
	test("form 400 marks every field, fills the slots and links them, without navigating", () => {
		const f = form(`
			<input name="title" aria-describedby="hint"><span data-tc-field-error="title"></span>
			<input name="n"><input name="body"><span data-tc-form-error></span>`);
		const submit = submitSpy(f);
		const title = control(f, '[name="title"]');
		const n = control(f, '[name="n"]');
		const slot = control(f, '[data-tc-field-error="title"]');
		const err = new commands.CommandError("note.capture", 400, { fields: { title: "a title is required", n: "must be a whole number" }, formError: "" });
		expect(forms.formFailure(f, err)).toBe("marked");
		expect([title.getAttribute("aria-invalid"), n.getAttribute("aria-invalid")]).toEqual(["true", "true"]);
		expect(slot.textContent).toBe("a title is required");
		expect(slot.id).not.toBe("");
		expect(title.getAttribute("aria-describedby")).toBe(`hint ${slot.id}`);
		expect(n.hasAttribute("aria-describedby")).toBe(false);
		expect(control(f, "[data-tc-form-error]").textContent).toBe("");
		expect(f.getAttribute("data-tc-error")).toBe("a title is required");
		expect(submit).not.toHaveBeenCalled();
		forms.clearFormErrors(f);
		expect(title.hasAttribute("aria-invalid") || n.hasAttribute("aria-invalid") || f.hasAttribute("data-tc-error")).toBe(false);
		expect(title.getAttribute("aria-describedby")).toBe("hint");
		expect(slot.textContent).toBe("");
	});

	test("form 400 with an error naming no field fills the form slot", () => {
		const f = form(`<span data-tc-form-error>rendered by the server</span>`);
		forms.formFailure(f, new commands.CommandError("x", 400, { formError: "nothing to add" }));
		expect(control(f, "[data-tc-form-error]").textContent).toBe("nothing to add");
		forms.clearFormErrors(f);
		expect(control(f, "[data-tc-form-error]").textContent).toBe("");
	});

	test("form 5xx falls back to the native POST exactly once", () => {
		const f = form("");
		const submit = submitSpy(f);
		expect(forms.formFailure(f, new commands.CommandError("x", 500))).toBe("fallback");
		expect(forms.formFailure(f, new commands.CommandError("x", 503))).toBe("ignored");
		expect(submit).toHaveBeenCalledTimes(1);
	});

	test("the fallback resubmits with the clicked button, so its name and value are posted", () => {
		const f = form(`<button name="intent" value="archive">archive</button>`);
		const submit = submitSpy(f);
		const button = control(f, "button");
		expect(forms.formFailure(f, new commands.CommandError("x", 500), button)).toBe("fallback");
		expect(submit).toHaveBeenCalledWith(button);
	});

	test("form network error falls back the same way", () => {
		const f = form("");
		const submit = submitSpy(f);
		expect(forms.formFailure(f, new commands.CommandError("x", 0))).toBe("fallback");
		expect(submit).toHaveBeenCalledTimes(1);
	});

	test("form 409 (session gone) falls back to the native submit once", () => {
		const f = form("");
		const submit = submitSpy(f);
		expect(forms.formFailure(f, new commands.CommandError("x", 409))).toBe("fallback");
		expect(forms.formFailure(f, new commands.CommandError("x", 409))).toBe("ignored");
		expect(submit).toHaveBeenCalledTimes(1);
	});
});

test("command errors carry status, fields and message", () => {
	const e = new commands.CommandError("a", 400, { fields: { f: "bad" } });
	expect(e).toBeInstanceOf(Error);
	expect([e.status, e.fields.f, e.formError, e.message]).toEqual([400, "bad", "", "bad"]);
	expect(new commands.CommandError("a", 400, { fields: { f: "bad" }, formError: "whole" }).message).toBe("whole");
	expect(new commands.CommandError("a", 500).message).toBe("live: command a failed (500)");
});

describe("multipart upload", () => {
	function payload(): FormData {
		const fd = new FormData();
		fd.append("note", "hello");
		fd.append("file", new File(["id,title\n1,x\n"], "a.csv", { type: "text/csv" }));
		return fd;
	}

	test("multipart upload posts the FormData to the action, never the JSON bus", async () => {
		tc.markReady();
		const calls = stubFetch({ status: 204 });
		await commands.postForm("import.upload", "/import", payload());
		expect(calls).toHaveLength(1);
		expect(calls[0].url).toBe("/import");
		expect(calls[0].init.method).toBe("POST");
		expect(calls[0].init.redirect).toBe("manual");
		expect(typeof calls[0].init.body).not.toBe("string");
		expect(calls[0].init.headers?.["X-TC-Sid"]).toBe("test-sid");
		expect(calls[0].init.headers).not.toHaveProperty("Content-Type");
		const sent = calls[0].init.body as unknown as FormData;
		const file = sent.get("file") as File;
		expect(file.name).toBe("a.csv");
		expect(await file.text()).toBe("id,title\n1,x\n");
		expect(sent.get("note")).toBe("hello");
	});

	test("multipart upload 400 rejects with the field for marking", async () => {
		tc.markReady();
		stubFetch({ status: 400, body: { error: { fields: { file: "too large" } } } });
		const err = await commands.postForm("import.upload", "/import", new FormData()).then(
			() => null,
			(e: unknown) => e,
		);
		expect(err).toBeInstanceOf(commands.CommandError);
		const ce = err as InstanceType<typeof commands.CommandError>;
		expect([ce.status, ce.fields.file, ce.message]).toEqual([400, "too large", "too large"]);
	});

	test.each([
		[{ status: 0, type: "opaqueredirect" }],
		[{ status: 303 }],
		[{ status: 500 }],
	])("multipart upload %j falls back", async (reply) => {
		tc.markReady();
		stubFetch(reply);
		const err = await commands.postForm("import.upload", "/import", new FormData()).then(
			() => null,
			(e: unknown) => e,
		);
		const f = form("");
		const submit = submitSpy(f);
		expect(forms.formFailure(f, err)).toBe("fallback");
		expect(submit).toHaveBeenCalledTimes(1);
	});
});

describe("send", () => {
	test("send answered 409 rejects: it reloads, or does not when the caller owns recovery", async () => {
		const reload = vi.spyOn(window.location, "reload").mockImplementation(() => {});
		tc.markReady();
		stubFetch({ status: 409 });
		const first = await commands.send("x.do").then(() => null, (e: unknown) => e);
		expect((first as InstanceType<typeof commands.CommandError>).status).toBe(409);
		expect(reload).toHaveBeenCalledTimes(1);
		const second = await commands.send("x.do", undefined, { noReload: true }).then(() => null, (e: unknown) => e);
		expect((second as InstanceType<typeof commands.CommandError>).status).toBe(409);
		expect(reload).toHaveBeenCalledTimes(1);
	});

	test("a send queued before ready rejects when a resync abandons it, and never fires", async () => {
		const reload = vi.spyOn(window.location, "reload").mockImplementation(() => {});
		const calls = stubFetch({ status: 204 });
		const queued = commands.send("x.do").then(() => null, (e: unknown) => e);
		live.apply({ kind: "resync" });
		const err = await queued;
		expect((err as InstanceType<typeof commands.CommandError>).status).toBe(0);
		expect(reload).toHaveBeenCalledTimes(1);
		tc.markReady();
		expect(calls).toHaveLength(0);
	});
});

describe("wireForms", () => {
	function submit(f: HTMLFormElement): boolean {
		const ev = new SubmitEvent("submit", { bubbles: true, cancelable: true });
		f.dispatchEvent(ev);
		return ev.defaultPrevented;
	}

	test("a form submit before ready goes native; once ready one command is in flight per form", async () => {
		const f = form(`<input name="title" value="t"><input type="hidden" name="_tc_submit" value="id1">`);
		f.dataset.tcForm = "note.capture";
		f.setAttribute("action", "/notes");
		const nativeSubmit = submitSpy(f);
		const field = control(f, '[name="_tc_submit"]') as HTMLInputElement;
		forms.wireForms();
		expect(submit(f)).toBe(false);

		tc.markReady();
		const gate = Promise.withResolvers<{ status: number; headers: Record<string, string> }>();
		const calls = stubFetch(gate.promise);
		expect(submit(f)).toBe(true);
		expect(submit(f)).toBe(true);
		expect(calls).toHaveLength(1);
		// posted to the form's own action, as natively, with the runtime's headers
		expect(calls[0].url).toBe("/notes");
		expect(calls[0].init.body).toBe("title=t&_tc_submit=id1");
		expect(calls[0].init.headers).toMatchObject({ "X-TC-Form": "1", "X-TC-Sid": "test-sid" });
		gate.resolve({ status: 204, headers: { "X-TC-Submit": "id2" } });
		await settle();
		expect(field.value).toBe("id2");
		expect(submit(f)).toBe(true);
		expect(calls).toHaveLength(2);
		expect(calls[1].init.body).toBe("title=t&_tc_submit=id2");
		expect(nativeSubmit).not.toHaveBeenCalled();
		f.setAttribute("data-tc-fallback", "");
		expect(submit(f)).toBe(false);
		expect(calls).toHaveLength(2);
	});
});

describe("errors that name no field", () => {
	const submit = (f: HTMLFormElement): boolean => !f.dispatchEvent(new SubmitEvent("submit", { bubbles: true, cancelable: true }));
	test("a 4xx's message fills the form error slot; a later success clears it", async () => {
		const f = form(`<input name="title" value="t"><span data-tc-form-error></span>`);
		f.dataset.tcForm = "note.edit";
		f.setAttribute("action", "/notes/999");
		forms.wireForms();
		tc.markReady();
		stubFetch({ status: 404, body: "no note 999" }, { status: 204 });
		submit(f);
		await settle();
		expect(f.querySelector("[data-tc-form-error]")!.textContent).toBe("no note 999");
		submit(f);
		await settle();
		expect(f.querySelector("[data-tc-form-error]")!.textContent).toBe("");
	});
});

describe("forms rendered without a submission id", () => {
	const submit = (f: HTMLFormElement): boolean => !f.dispatchEvent(new SubmitEvent("submit", { bubbles: true, cancelable: true }));
	const sentKey = (body: unknown): string => new URLSearchParams(body as string).get("_tc_submit") ?? "";

	test("carry a runtime key, kept for a retry, then the server's id", async () => {
		const f = form(`<input name="title" value="t">`);
		f.dataset.tcForm = "note.add";
		f.setAttribute("action", "/notes");
		forms.wireForms();
		tc.markReady();
		const calls = stubFetch({ status: 422, body: "no" }, { status: 204, headers: { "X-TC-Submit": "server-id" } }, { status: 204 });
		submit(f);
		await settle();
		submit(f);
		await settle();
		submit(f);
		await settle();
		const [first, retry, next] = calls.map((c) => sentKey(c.init.body));
		expect(first).toMatch(/^c\.[0-9a-f]{32}$/);
		expect(retry).toBe(first);
		expect(next).toBe("server-id");
	});

	test("send({ once }) carries the key on the command bus", async () => {
		tc.markReady();
		const calls = stubFetch({ status: 204 });
		const key = commands.submitKey();
		await commands.send("signup.register", { user: "ada" }, { submit: key });
		expect(calls[0].json().submit).toBe(key);
	});
});
